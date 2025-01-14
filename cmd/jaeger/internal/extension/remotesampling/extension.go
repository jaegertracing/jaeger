// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/leaderelection"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	samplinggrpc "github.com/jaegertracing/jaeger/internal/sampling/grpc"
	samplinghttp "github.com/jaegertracing/jaeger/internal/sampling/http"
	"github.com/jaegertracing/jaeger/internal/sampling/strategy"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/static"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

var _ extension.Extension = (*rsExtension)(nil)

// type Extension interface {
// 	extension.Extension
// 	// rs *rsExtension
// }

const defaultResourceName = "sampling_store_leader"

type rsExtension struct {
	cfg              *Config
	telemetry        component.TelemetrySettings
	httpServer       *http.Server
	grpcServer       *grpc.Server
	strategyProvider strategy.Provider // TODO we should rename this to Provider, not "store"
	adaptiveStore    samplingstore.Store
	distLock         *leaderelection.DistributedElectionParticipant
	shutdownWG       sync.WaitGroup
}

func newExtension(cfg *Config, telemetry component.TelemetrySettings) *rsExtension {
	return &rsExtension{
		cfg:       cfg,
		telemetry: telemetry,
	}
}

// AdaptiveSamplingComponents is a struct that holds the components needed for adaptive sampling.
type AdaptiveSamplingComponents struct {
	SamplingStore samplingstore.Store
	DistLock      *leaderelection.DistributedElectionParticipant
	Options       *adaptive.Options
}

// GetAdaptiveSamplingComponents locates the `remotesampling` extension in Host
// and returns the sampling store and a loader/follower implementation, provided
// that the extension is configured with adaptive sampling (vs. file-based config).
func GetAdaptiveSamplingComponents(host component.Host) (*AdaptiveSamplingComponents, error) {
	var comp component.Component
	var compID component.ID
	for id, ext := range host.GetExtensions() {
		if id.Type() == ComponentType {
			comp = ext
			compID = id
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			ComponentType,
		)
	}
	ext, ok := comp.(*rsExtension)
	if !ok {
		return nil, fmt.Errorf("extension '%s' is not of type '%s'", compID, ComponentType)
	}
	if ext.adaptiveStore == nil || ext.distLock == nil {
		return nil, fmt.Errorf("extension '%s' is not configured for adaptive sampling", compID)
	}
	return &AdaptiveSamplingComponents{
		SamplingStore: ext.adaptiveStore,
		DistLock:      ext.distLock,
		Options:       &ext.cfg.Adaptive.Options,
	}, nil
}

func (ext *rsExtension) Start(ctx context.Context, host component.Host) error {
	if ext.cfg.File != nil {
		ext.telemetry.Logger.Info(
			"Starting file-based sampling strategy provider",
			zap.String("path", ext.cfg.File.Path),
		)
		if err := ext.startFileBasedStrategyProvider(ctx); err != nil {
			return err
		}
	}

	if ext.cfg.Adaptive != nil {
		ext.telemetry.Logger.Info(
			"Starting adaptive sampling strategy provider",
			zap.String("sampling_store", ext.cfg.Adaptive.SamplingStore),
		)
		if err := ext.startAdaptiveStrategyProvider(host); err != nil {
			return err
		}
	}

	if ext.cfg.HTTP != nil {
		if err := ext.startHTTPServer(ctx, host); err != nil {
			return fmt.Errorf("failed to start sampling http server: %w", err)
		}
	}

	if ext.cfg.GRPC != nil {
		if err := ext.startGRPCServer(ctx, host); err != nil {
			return fmt.Errorf("failed to start sampling gRPC server: %w", err)
		}
	}

	return nil
}

func (ext *rsExtension) Shutdown(ctx context.Context) error {
	var errs []error

	if ext.httpServer != nil {
		if err := ext.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop the sampling HTTP server: %w", err))
		}
	}

	if ext.grpcServer != nil {
		ext.grpcServer.GracefulStop()
	}

	if ext.distLock != nil {
		if err := ext.distLock.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop the distributed lock: %w", err))
		}
	}

	if ext.strategyProvider != nil {
		if err := ext.strategyProvider.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop strategy provider: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (ext *rsExtension) startFileBasedStrategyProvider(_ context.Context) error {
	opts := static.Options{
		StrategiesFile:             ext.cfg.File.Path,
		ReloadInterval:             ext.cfg.File.ReloadInterval,
		IncludeDefaultOpStrategies: includeDefaultOpStrategies.IsEnabled(),
		DefaultSamplingProbability: ext.cfg.File.DefaultSamplingProbability,
	}

	// contextcheck linter complains about next line that context is not passed.
	//nolint
	provider, err := static.NewProvider(opts, ext.telemetry.Logger)
	if err != nil {
		return fmt.Errorf("failed to create the local file strategy store: %w", err)
	}

	ext.strategyProvider = provider
	return nil
}

func (ext *rsExtension) startAdaptiveStrategyProvider(host component.Host) error {
	storageName := ext.cfg.Adaptive.SamplingStore
	f, err := jaegerstorage.GetStorageFactory(storageName, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	storeFactory, ok := f.(storage.SamplingStoreFactory)
	if !ok {
		return fmt.Errorf("storage '%s' does not support sampling store", storageName)
	}

	store, err := storeFactory.CreateSamplingStore(ext.cfg.Adaptive.AggregationBuckets)
	if err != nil {
		return fmt.Errorf("failed to create the sampling store: %w", err)
	}
	ext.adaptiveStore = store

	{
		lock, err := storeFactory.CreateLock()
		if err != nil {
			return fmt.Errorf("failed to create the distributed lock: %w", err)
		}

		ep := leaderelection.NewElectionParticipant(lock, defaultResourceName,
			leaderelection.ElectionParticipantOptions{
				LeaderLeaseRefreshInterval:   ext.cfg.Adaptive.LeaderLeaseRefreshInterval,
				FollowerLeaseRefreshInterval: ext.cfg.Adaptive.FollowerLeaseRefreshInterval,
				Logger:                       ext.telemetry.Logger,
			})
		if err := ep.Start(); err != nil {
			return fmt.Errorf("failed to start the leader election participant: %w", err)
		}
		ext.distLock = ep
	}

	provider := adaptive.NewProvider(ext.cfg.Adaptive.Options, ext.telemetry.Logger, ext.distLock, store)
	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start the adaptive strategy store: %w", err)
	}
	ext.strategyProvider = provider
	return nil
}

func (ext *rsExtension) startHTTPServer(ctx context.Context, host component.Host) error {
	mf := otelmetrics.NewFactory(ext.telemetry.MeterProvider)
	mf = mf.Namespace(metrics.NSOptions{Name: "jaeger_remote_sampling"})
	handler := samplinghttp.NewHandler(samplinghttp.HandlerParams{
		ConfigManager: &samplinghttp.ConfigManager{
			SamplingProvider: ext.strategyProvider,
		},
		MetricsFactory: mf,

		// In v1 the sampling endpoint in the collector was at /api/sampling, because
		// the collector reused the same port for multiple services. In v2, the extension
		// always uses a separate port, making /api prefix unnecessary.
		BasePath: "",
	})
	httpMux := http.NewServeMux()
	handler.RegisterRoutesWithHTTP(httpMux)

	var err error
	if ext.httpServer, err = ext.cfg.HTTP.ToServer(ctx, host, ext.telemetry, httpMux); err != nil {
		return err
	}

	ext.telemetry.Logger.Info(
		"Starting remote sampling HTTP server",
		zap.String("endpoint", ext.cfg.HTTP.Endpoint),
	)
	var hln net.Listener
	if hln, err = ext.cfg.HTTP.ToListener(ctx); err != nil {
		return err
	}

	ext.shutdownWG.Add(1)
	go func() {
		defer ext.shutdownWG.Done()

		err := ext.httpServer.Serve(hln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

func (ext *rsExtension) startGRPCServer(ctx context.Context, host component.Host) error {
	var err error
	if ext.grpcServer, err = ext.cfg.GRPC.ToServer(ctx, host, ext.telemetry); err != nil {
		return err
	}

	api_v2.RegisterSamplingManagerServer(ext.grpcServer, samplinggrpc.NewHandler(ext.strategyProvider))

	healthServer := health.NewServer() // support health checks on the gRPC server
	healthServer.SetServingStatus("jaeger.api_v2.SamplingManager", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(ext.grpcServer, healthServer)

	ext.telemetry.Logger.Info(
		"Starting remote sampling GRPC server",
		zap.String("endpoint", ext.cfg.GRPC.NetAddr.Endpoint),
	)
	var gln net.Listener
	if gln, err = ext.cfg.GRPC.NetAddr.Listen(ctx); err != nil {
		return err
	}

	ext.shutdownWG.Add(1)
	go func() {
		defer ext.shutdownWG.Done()
		if err := ext.grpcServer.Serve(gln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

func (*rsExtension) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

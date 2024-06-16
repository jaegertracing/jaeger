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
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/adaptive"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

var _ extension.Extension = (*rsExtension)(nil)

const defaultResourceName = "sampling_store_leader"

type rsExtension struct {
	cfg              *Config
	telemetry        component.TelemetrySettings
	httpServer       *http.Server
	grpcServer       *grpc.Server
	strategyProvider strategystore.StrategyStore // TODO we should rename this to Provider, not "store"
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

// GetAdaptiveSamplingStore locates the `remotesampling` extension in Host
// and returns the sampling store and a loader/follower implementation, provided
// that the extension is configured with adaptive sampling (vs. file-based config).
func GetAdaptiveSamplingStore(
	host component.Host,
) (samplingstore.Store, *leaderelection.DistributedElectionParticipant, error) {
	var comp component.Component
	var compID component.ID
	for id, ext := range host.GetExtensions() {
		if id.Type() == componentType {
			comp = ext
			compID = id
			break
		}
	}
	if comp == nil {
		return nil, nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			componentType,
		)
	}
	ext, ok := comp.(*rsExtension)
	if !ok {
		return nil, nil, fmt.Errorf("extension '%s' is not of type '%s'", compID, componentType)
	}
	if ext.adaptiveStore == nil || ext.distLock == nil {
		return nil, nil, fmt.Errorf("extension '%s' is not configured for adaptive sampling", compID)
	}
	return ext.adaptiveStore, ext.distLock, nil
}

func (ext *rsExtension) Start(ctx context.Context, host component.Host) error {
	if ext.cfg.File.Path != "" {
		if err := ext.startFileStrategyStore(ctx); err != nil {
			return err
		}
	}

	if ext.cfg.Adaptive.StrategyStore != "" {
		if err := ext.startAdaptiveStrategyStore(host); err != nil {
			return err
		}
	}

	if err := ext.startHTTPServer(ctx, host); err != nil {
		return fmt.Errorf("failed to start sampling http server: %w", err)
	}

	if err := ext.startGRPCServer(ctx, host); err != nil {
		return fmt.Errorf("failed to start sampling gRPC server: %w", err)
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

func (ext *rsExtension) startFileStrategyStore(_ context.Context) error {
	opts := static.Options{
		StrategiesFile: ext.cfg.File.Path,
	}

	// contextcheck linter complains about next line that context is not passed.
	//nolint
	provider, err := static.NewStrategyStore(opts, ext.telemetry.Logger)
	if err != nil {
		return fmt.Errorf("failed to create the local file strategy store: %w", err)
	}

	ext.strategyProvider = provider
	return nil
}

func (ext *rsExtension) startAdaptiveStrategyStore(host component.Host) error {
	storageName := ext.cfg.Adaptive.StrategyStore
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

	// TODO it is unlikely all these options are needed, we should refactor more
	opts := adaptive.Options{
		InitialSamplingProbability:   ext.cfg.Adaptive.InitialSamplingProbability,
		MinSamplesPerSecond:          ext.cfg.Adaptive.MinSamplesPerSecond,
		LeaderLeaseRefreshInterval:   ext.cfg.Adaptive.LeaderLeaseRefreshInterval,
		FollowerLeaseRefreshInterval: ext.cfg.Adaptive.FollowerLeaseRefreshInterval,
		AggregationBuckets:           ext.cfg.Adaptive.AggregationBuckets,
	}
	provider := adaptive.NewStrategyStore(opts, ext.telemetry.Logger, ext.distLock, store)
	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start the adaptive strategy store: %w", err)
	}
	ext.strategyProvider = provider
	return nil
}

func (ext *rsExtension) startGRPCServer(ctx context.Context, host component.Host) error {
	var err error
	if ext.grpcServer, err = ext.cfg.GRPC.ToServer(ctx, host, ext.telemetry); err != nil {
		return err
	}

	healthServer := health.NewServer()
	api_v2.RegisterSamplingManagerServer(ext.grpcServer, sampling.NewGRPCHandler(ext.strategyProvider))
	healthServer.SetServingStatus("jaeger.api_v2.SamplingManager", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(ext.grpcServer, healthServer)
	ext.telemetry.Logger.Info("Starting GRPC server", zap.String("endpoint", ext.cfg.GRPC.NetAddr.Endpoint))

	var gln net.Listener
	if gln, err = ext.cfg.GRPC.NetAddr.Listen(ctx); err != nil {
		return err
	}

	ext.shutdownWG.Add(1)
	go func() {
		defer ext.shutdownWG.Done()

		if errGrpc := ext.grpcServer.Serve(gln); errGrpc != nil && !errors.Is(errGrpc, grpc.ErrServerStopped) {
			ext.telemetry.ReportStatus(component.NewFatalErrorEvent(errGrpc))
		}
	}()

	return nil
}

func (ext *rsExtension) startHTTPServer(ctx context.Context, host component.Host) error {
	httpMux := http.NewServeMux()

	handler := clientcfghttp.NewHTTPHandler(clientcfghttp.HTTPHandlerParams{
		ConfigManager: &clientcfghttp.ConfigManager{
			SamplingStrategyStore: ext.strategyProvider,
		},
		MetricsFactory: metrics.NullFactory,
		BasePath:       "/api",
	})

	handler.RegisterRoutesWithHTTP(httpMux)

	var err error
	if ext.httpServer, err = ext.cfg.HTTP.ToServer(ctx, host, ext.telemetry, httpMux); err != nil {
		return err
	}

	ext.telemetry.Logger.Info("Starting HTTP server", zap.String("endpoint", ext.cfg.HTTP.ServerConfig.Endpoint))
	var hln net.Listener
	if hln, err = ext.cfg.HTTP.ServerConfig.ToListener(ctx); err != nil {
		return err
	}

	ext.shutdownWG.Add(1)
	go func() {
		defer ext.shutdownWG.Done()

		err := ext.httpServer.Serve(hln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			ext.telemetry.ReportStatus(component.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

func (*rsExtension) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

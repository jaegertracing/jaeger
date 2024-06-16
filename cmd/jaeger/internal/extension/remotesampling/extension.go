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
	cfg           *Config
	telemetry     component.TelemetrySettings
	httpServer    *http.Server
	grpcServer    *grpc.Server
	strategyStore strategystore.StrategyStore
	shutdownWG    sync.WaitGroup
}

func newExtension(cfg *Config, telemetry component.TelemetrySettings) *rsExtension {
	return &rsExtension{
		cfg:       cfg,
		telemetry: telemetry,
	}
}

func GetExtensionConfig(host component.Host) (AdaptiveConfig, error) {
	var comp component.Component
	for id, ext := range host.GetExtensions() {
		if id.Type() == componentType {
			comp = ext
			break
		}
	}
	if comp == nil {
		return AdaptiveConfig{}, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			componentType,
		)
	}
	return comp.(*rsExtension).cfg.Adaptive, nil
}

// GetSamplingStorage retrieves a storage factory from `jaegerstorage` extension
// and uses it to create a sampling store and a loader/follower implementation.
func GetSamplingStorage(
	samplingStorage string,
	host component.Host,
	opts adaptive.Options,
	logger *zap.Logger,
) (samplingstore.Store, *leaderelection.DistributedElectionParticipant, error) {
	f, err := jaegerstorage.GetStorageFactory(samplingStorage, host)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot find storage factory: %w", err)
	}

	ssStore, ok := f.(storage.SamplingStoreFactory)
	if !ok {
		return nil, nil, fmt.Errorf("storage factory of type %s does not support sampling store", samplingStorage)
	}

	lock, err := ssStore.CreateLock()
	if err != nil {
		return nil, nil, err
	}

	store, err := ssStore.CreateSamplingStore(opts.AggregationBuckets)
	if err != nil {
		return nil, nil, err
	}

	ep := leaderelection.NewElectionParticipant(lock, defaultResourceName, leaderelection.ElectionParticipantOptions{
		FollowerLeaseRefreshInterval: opts.FollowerLeaseRefreshInterval,
		LeaderLeaseRefreshInterval:   opts.LeaderLeaseRefreshInterval,
		Logger:                       logger,
	})

	return store, ep, nil
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

	if ext.strategyStore != nil {
		if err := ext.strategyStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop strategy store: %w", err))
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
	ss, err := static.NewStrategyStore(opts, ext.telemetry.Logger)
	if err != nil {
		return fmt.Errorf("failed to create the local file strategy store: %w", err)
	}

	ext.strategyStore = ss
	return nil
}

func (ext *rsExtension) startAdaptiveStrategyStore(host component.Host) error {
	opts := adaptive.Options{
		InitialSamplingProbability:   ext.cfg.Adaptive.InitialSamplingProbability,
		MinSamplesPerSecond:          ext.cfg.Adaptive.MinSamplesPerSecond,
		LeaderLeaseRefreshInterval:   ext.cfg.Adaptive.LeaderLeaseRefreshInterval,
		FollowerLeaseRefreshInterval: ext.cfg.Adaptive.FollowerLeaseRefreshInterval,
		AggregationBuckets:           ext.cfg.Adaptive.AggregationBuckets,
	}

	store, ep, err := GetSamplingStorage(ext.cfg.Adaptive.StrategyStore, host, opts, ext.telemetry.Logger)
	if err != nil {
		return err
	}

	ss := adaptive.NewStrategyStore(opts, ext.telemetry.Logger, ep, store)
	ss.Start()
	ext.strategyStore = ss
	return nil
}

func (ext *rsExtension) startGRPCServer(ctx context.Context, host component.Host) error {
	var err error
	if ext.grpcServer, err = ext.cfg.GRPC.ToServer(ctx, host, ext.telemetry); err != nil {
		return err
	}

	healthServer := health.NewServer()
	api_v2.RegisterSamplingManagerServer(ext.grpcServer, sampling.NewGRPCHandler(ext.strategyStore))
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
			SamplingStrategyStore: ext.strategyStore,
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

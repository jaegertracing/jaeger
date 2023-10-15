// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package allinone

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/extensions"
	"go.opentelemetry.io/collector/service/pipelines"
	"go.opentelemetry.io/collector/service/telemetry"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/extension/jaegerstorage"
)

type configProvider struct {
	watcher chan error
}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

// NewConfigProvider creates a new ConfigProvider.
func NewConfigProvider() *configProvider {
	return &configProvider{
		watcher: make(chan error, 1),
	}
}

func (cp *configProvider) Get(ctx context.Context, factories otelcol.Factories) (*otelcol.Config, error) {
	cfg := &otelcol.Config{
		Service:    cp.makeServiceConfig(),
		Extensions: make(map[component.ID]component.Config),
		Receivers:  make(map[component.ID]component.Config),
		Processors: make(map[component.ID]component.Config),
		Exporters:  make(map[component.ID]component.Config),
	}
	defaultConfigs("extension", cfg.Service.Extensions, cfg.Extensions, factories.Extensions)
	for _, pipeCfg := range cfg.Service.Pipelines {
		defaultConfigs("receiver", pipeCfg.Receivers, cfg.Receivers, factories.Receivers)
		defaultConfigs("processor", pipeCfg.Processors, cfg.Processors, factories.Processors)
		defaultConfigs("exporter", pipeCfg.Exporters, cfg.Exporters, factories.Exporters)
	}
	return cfg, nil
}

func defaultConfigs[TFactory component.Factory](
	componentType string,
	comps []component.ID,
	outCfg map[component.ID]component.Config,
	factories map[component.Type]TFactory,
) error {
	for _, compID := range comps {
		f, ok := factories[compID.Type()]
		if !ok {
			return fmt.Errorf("no factory registered for %s %v", componentType, compID)
		}
		cfg := f.CreateDefaultConfig()
		outCfg[compID] = cfg
	}
	return nil
}

// makeServiceConfig creates default config that contains
// all standard all-in-one extensions and pipelines.
func (cp *configProvider) makeServiceConfig() service.Config {
	return service.Config{
		Extensions: extensions.Config([]component.ID{
			jaegerstorage.ID,
			jaegerquery.ID,
		}),
		Pipelines: pipelines.Config(map[component.ID]*pipelines.PipelineConfig{
			component.NewID("traces"): {
				Receivers: []component.ID{
					component.NewID("otlp"),
					component.NewID("jaeger"),
					component.NewID("zipkin"),
				},
				Processors: []component.ID{
					component.NewID("batch"),
				},
				Exporters: []component.ID{
					storageexporter.ID,
				},
			},
		}),
		// OTel Collector currently (v0.87) hardcodes telemetry settings, this is a copy.
		// https://github.com/open-telemetry/opentelemetry-collector/blob/35512c466577036b0cc306673d2d4ad039c77f1c/otelcol/unmarshaler.go#L43
		Telemetry: telemetry.Config{
			Logs: telemetry.LogsConfig{
				Level:       zapcore.InfoLevel,
				Development: false,
				Encoding:    "console",
				Sampling: &telemetry.LogsSamplingConfig{
					Enabled:    true,
					Tick:       10 * time.Second,
					Initial:    10,
					Thereafter: 100,
				},
				OutputPaths:       []string{"stderr"},
				ErrorOutputPaths:  []string{"stderr"},
				DisableCaller:     false,
				DisableStacktrace: false,
				InitialFields:     map[string]any(nil),
			},
			Metrics: telemetry.MetricsConfig{
				Level: configtelemetry.LevelNone,
				// Address: ":8888",
			},
			// TODO initialize tracer
		},
	}
}

// Watch implements otelcol.ConfigProvider.
// The returned channel is never written to, as there is no configuration to watch.
func (cp *configProvider) Watch() <-chan error {
	return cp.watcher
}

// Shutdown implements otelcol.ConfigProvider.
func (cp *configProvider) Shutdown(ctx context.Context) error {
	close(cp.watcher)
	return nil
}

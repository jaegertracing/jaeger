package healthcheckextension

import (
	"context"

	"github.com/jaegertracing/jaeger/ports"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
)

const defaultPort = 13133

// ComponentType is the name of this extension in configuration
var (
	ComponentType = component.MustNewType("healthcheck")
	ID            = component.NewID(ComponentType)
)

// New Factory creates a factory for health check extension
func NewFactory() extension.Factory {
	return extension.NewFactory(
		ComponentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelBeta,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		ServerConfig: confighttp.ServerConfig{
			Endpoint: ports.PortToHostPort(defaultPort),
		},
		CheckCollectorPipeline: defaultCheckCollectorPipelineSettings(),
		Path:                   "/",
	}
}

// defaultCheckCollectorPipelineSettings returns the default settings for CheckCollectorPipeline.
func defaultCheckCollectorPipelineSettings() checkCollectorPipelineSettings {
	return checkCollectorPipelineSettings{
		Enabled:                  false,
		Interval:                 "5m",
		ExporterFailureThreshold: 5,
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := cfg.(*Config)
	return newServer(*config, set.TelemetrySettings), nil
}

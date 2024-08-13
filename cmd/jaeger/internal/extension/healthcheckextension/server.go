package healthcheckextension

import (
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

type healthCheckExtension struct {
	config   Config
	logger   *zap.Logger
	state    *healthcheck.HealthCheck
	settings component.TelemetrySettings
}

func newServer(cfg Config, set component.TelemetrySettings) extension.Extension {
	hc := &healthCheckExtension{
		config:   cfg,
		logger:   set.Logger,
		state:    healthcheck.New(),
		settings: set,
	}

	hc.state.SetLogger(set.Logger)

	return hc
}

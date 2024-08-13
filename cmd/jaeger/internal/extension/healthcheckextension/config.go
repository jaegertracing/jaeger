package healthcheckextension

import (
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
)

type ResponseBodySettings struct {
	// Healthy represents the body of the response returned when the collector is healthy.
	// The default value is ""
	Healthy string `mapstructure:"healthy"`

	// Unhealthy represents the body of the response returned when the collector is unhealthy.
	// The default value is ""
	Unhealthy string `mapstructure:"unhealthy"`
}

// Config has the configuration for the extension enabling the health check
// extension, used to report the health status of the service.
type Config struct {
	confighttp.ServerConfig `mapstructure:",squash"`

	// Path represents the path the health check service will serve.
	// The default path is "/".
	Path string `mapstructure:"path"`

	// ResponseBody represents the body of the response returned by the health check service.
	// This overrides the default response that it would return.
	ResponseBody *ResponseBodySettings `mapstructure:"response_body"`

	// CheckCollectorPipeline contains the list of settings of collector pipeline health check
	CheckCollectorPipeline checkCollectorPipelineSettings `mapstructure:"check_collector_pipeline"`
}

var _ component.Config = (*Config)(nil)
var (
	errNoEndpointProvided                      = errors.New("bad config: endpoint must be specified")
	errInvalidExporterFailureThresholdProvided = errors.New("bad config: exporter_failure_threshold expects a positive number")
	errInvalidPath                             = errors.New("bad config: path must start with /")
)

// Validate checks if the extension configuration is valid
func (cfg *Config) Validate() error {
	_, err := time.ParseDuration(cfg.CheckCollectorPipeline.Interval)
	if err != nil {
		return err
	}
	if cfg.Endpoint == "" {
		return errNoEndpointProvided
	}
	if cfg.CheckCollectorPipeline.ExporterFailureThreshold <= 0 {
		return errInvalidExporterFailureThresholdProvided
	}
	if !strings.HasPrefix(cfg.Path, "/") {
		return errInvalidPath
	}
	return nil
}

type checkCollectorPipelineSettings struct {
	// Enabled indicates whether to not enable collector pipeline check.
	Enabled bool `mapstructure:"enabled"`
	// Interval the time range to check healthy status of collector pipeline
	Interval string `mapstructure:"interval"`
	// ExporterFailureThreshold is the threshold of exporter failure numbers during the Interval
	ExporterFailureThreshold int `mapstructure:"exporter_failure_threshold"`
}

package grpc

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/tenancy"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

type Config struct {
	Tenancy                      tenancy.Options `mapstructure:"multi_tenancy"`
	configgrpc.ClientConfig      `mapstructure:",squash"`
	exporterhelper.TimeoutConfig `mapstructure:",squash"`
	enabled                      bool
}

func DefaultConfig() Config {
	return Config{
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: defaultConnectionTimeout,
		},
	}
}

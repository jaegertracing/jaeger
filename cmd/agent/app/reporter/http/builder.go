package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/http/client"
	"go.uber.org/zap"
)

const (
	defaultRequestTimeout = time.Second * 5
)

// Builder struct to hold configuration
type Builder struct {
	// the full url of HTTP endpoint of collector
	// e.g. http://<domain_name>/<path>
	CollectorEndpoint string        `yaml:"collectorEndpoint"`
	RequestTimeout    time.Duration `yaml:"requestTimeout"`
	Logger            *zap.Logger
	Client            client.Client
}

// NewBuilder create a new empty instance of Builder
func NewBuilder() *Builder {
	return &Builder{
		RequestTimeout: defaultRequestTimeout,
	}
}

// WithCollectorEndpoint set collector endpoint value
func (b *Builder) WithCollectorEndpoint(collectorEndpoint string) *Builder {
	b.CollectorEndpoint = collectorEndpoint
	return b
}

// WithRequestTimeout set request timeout value
func (b *Builder) WithRequestTimeout(requestTimeout time.Duration) *Builder {
	b.RequestTimeout = requestTimeout
	return b
}

// WithLogger set logger
func (b *Builder) WithLogger(logger *zap.Logger) *Builder {
	b.Logger = logger
	return b
}

// CreateReporter create new instance of Reporter
func (b *Builder) CreateReporter() (*Reporter, error) {
	if b.CollectorEndpoint == "" {
		return nil, errors.New("Invalid Parameter CollectorEndpoint, Non-empty onme required")
	}
	if b.Logger == nil {
		b.Logger = zap.NewNop()
	}

	if b.Client == nil {
		b.Client = &client.SimpleClient{
			Endpoint: b.CollectorEndpoint,
			Cli: &http.Client{
				Timeout: b.RequestTimeout,
			},
		}
	}
	return New(b.Client, b.CollectorEndpoint, b.Logger), nil
}

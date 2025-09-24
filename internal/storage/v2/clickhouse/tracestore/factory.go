package tracestore

import (
	"context"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
)

type Factory struct {
	config Config
	telset telemetry.Settings
	conn   driver.Conn
}

func NewFactory(ctx context.Context, cfg Config, telset telemetry.Settings) (*Factory, error) {
	f := &Factory{
		config: cfg,
		telset: telset,
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: f.config.Addresses,
		Auth: clickhouse.Auth{
			Database: f.config.Auth.Database,
			Username: f.config.Auth.Username,
			Password: f.config.Auth.Password,
		},
		DialTimeout: f.config.DialTimeout,
	})
	if err != nil {
		defer conn.Close()
		return nil, fmt.Errorf("failed to create ClickHouse connection: %w", err)
	}
	err = conn.Ping(ctx)
	if err != nil {
		defer conn.Close()
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}
	f.conn = conn
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return NewReader(f.conn), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return NewWriter(f.conn), nil
}

func (f *Factory) Close() error {
	if err := f.conn.Close(); err != nil {
		return fmt.Errorf("failed to close ClickHouse connection: %w", err)
	}
	return nil
}

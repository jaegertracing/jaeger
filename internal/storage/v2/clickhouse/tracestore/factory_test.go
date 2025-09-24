package tracestore

import (
	"context"
	"testing"

	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestNewFactory(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:latest",
		ExposedPorts: []string{"9000:9000"},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      "./testdata/clickhouse-users.xml",
				ContainerFilePath: "/etc/clickhouse-server/users.d/jaeger-users.xml",
				FileMode:          0o644,
			},
		},
		WaitingFor: wait.ForListeningPort("9000"),
	}
	clickhouseC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	testcontainers.CleanupContainer(t, clickhouseC)
	require.NoError(t, err)

	factory, err := NewFactory(context.Background(), Config{
		Addresses: []string{"localhost:9000"},
		Auth: AuthConfig{
			Database: "default",
			Username: "default",
			Password: "jaeger",
		},
	}, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NotNil(t, factory)
	require.NoError(t, factory.Close())
}

func TestNewFactory_FailedPing(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:latest",
		ExposedPorts: []string{"9000:9000"},
		WaitingFor:   wait.ForListeningPort("9000"),
	}
	clickhouseC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	testcontainers.CleanupContainer(t, clickhouseC)
	require.NoError(t, err)

	factory, err := NewFactory(context.Background(), Config{
		Addresses: []string{"localhost:9999"}, // wrong port
		Auth: AuthConfig{
			Database: "default",
			Username: "default",
			Password: "jaeger",
		},
	}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "failed to ping ClickHouse")
	require.Nil(t, factory)
}

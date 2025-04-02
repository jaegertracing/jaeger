package grpc

import (
	"log"
	"net"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"google.golang.org/grpc"
)

func TestNewFactory_NonEmptyAuthenticator(t *testing.T) {
	cfg := &Config{
		ClientConfig: configgrpc.ClientConfig{
			Auth: &configauth.Authentication{},
		},
	}
	_, err := NewFactory(*cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "authenticator is not supported")
}

func TestNewFactory(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")

	s := grpc.NewServer()
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
	t.Cleanup(s.Stop)

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 1 * time.Second,
		},
		Tenancy: tenancy.Options{
			Enabled: true,
		},
	}
	telset := telemetry.NoopSettings()
	f, err := NewFactory(cfg, telset)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

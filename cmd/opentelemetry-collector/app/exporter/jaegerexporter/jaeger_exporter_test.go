package jaegerexporter

import (
	"context"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/exporter/jaegerexporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path"
	"testing"

	grpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(grpcRep.AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: &jaegerexporter.Factory{}}
	cfg := factory.CreateDefaultConfig().(*jaegerexporter.Config)
	assert.Empty(t, cfg.GRPCSettings.Endpoint)
	tlsConf := cfg.TLSConfig
	assert.False(t, tlsConf.UseSecure)
	assert.Empty(t, tlsConf.CaCert)
	assert.Empty(t, tlsConf.ClientKey)
	assert.Empty(t, tlsConf.ClientCert)
	assert.Empty(t, tlsConf.ServerNameOverride)
}

func TestDefaultValueFromViper(t *testing.T) {
	v, c := jConfig.Viperize(grpcRep.AddFlags)
	err := c.ParseFlags([]string{"--reporter.grpc.host-port=foo", "--reporter.grpc.tls.enabled=true", "--reporter.grpc.tls.ca=ca.crt"})
	require.NoError(t, err)

	f := &Factory{
		Wrapped: &jaegerexporter.Factory{},
		Viper:   v,
	}

	cfg := f.CreateDefaultConfig().(*jaegerexporter.Config)
	assert.Equal(t, "foo", cfg.GRPCSettings.Endpoint)
	tlsConfig := cfg.TLSConfig
	assert.Equal(t, true, tlsConfig.UseSecure)
	assert.Equal(t, "ca.crt", tlsConfig.CaCert)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := config.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(grpcRep.AddFlags)
	err = c.ParseFlags([]string{"--reporter.grpc.host-port=foo"})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: &jaegerexporter.Factory{}}
	assert.Equal(t, "foo", factory.CreateDefaultConfig().(*jaegerexporter.Config).GRPCSettings.Endpoint)

	factories.Exporters["jaeger"] = factory
	colConfig, err := config.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters["jaeger"].(*jaegerexporter.Config)
	assert.Equal(t, "bar", cfg.GRPCSettings.Endpoint)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: &jaegerexporter.Factory{},
	}
	assert.Equal(t, configmodels.Type("jaeger"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: &jaegerexporter.Factory{},
	}
	mReceiver, err := f.CreateMetricsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	assert.Equal(t, configerror.ErrDataTypeIsNotSupported, err)
	assert.Nil(t, mReceiver)
}

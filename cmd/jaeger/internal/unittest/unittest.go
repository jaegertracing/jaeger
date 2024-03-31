package unittest

import (
	"context"
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/service/telemetry"
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

type StorageTest struct {
	Name       string
	ConfigFile string
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[jaegerstorage.ID] = host.storageExtension
	return myMap
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (host storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	host.t.Log("Calling Factory")
	return nil
}

func (host storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	host.t.Log("Calling Exporters")
	return nil
}

func (s *StorageTest) Test(t *testing.T) {
	if os.Getenv("STORAGE") != s.Name {
		t.Skipf("Integration test against Jaeger-V2 %[1]s skipped; set STORAGE env var to %[1]s to run this", s.Name)
	}
	var v integration.StorageIntegration
	// v.
	factories, err := internal.Components()
	require.NoError(t, err)
	fmp := fileprovider.New()
	configProvider, err := otelcol.NewConfigProvider(
		otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs:      []string{s.ConfigFile},
				Providers: map[string]confmap.Provider{fmp.Scheme(): fmp},
			},
		},
	)
	require.NoError(t, err)
	cfg, err := configProvider.Get(context.Background(), factories)
	require.NoError(t, err)
	tel, err := telemetry.New(context.Background(), telemetry.Settings{}, cfg.Service.Telemetry)
	require.NoError(t, err)
	config, err := os.ReadFile(s.ConfigFile)
	require.NoError(t, err)
	t.Log(string(config))
	storageCfg, ok := cfg.Extensions[jaegerstorage.ID].(*jaegerstorage.Config)
	require.True(t, ok, "no jaeger storage extension found in the config")
	exporterCfg, ok := cfg.Exporters[storageexporter.ID].(*storageexporter.Config)
	require.True(t, ok, "no jaeger storage exporter found in the config")

	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = tel.Logger()

	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.CreateSettings{
			ID:                jaegerstorage.ID,
			TelemetrySettings: telemetrySettings,
		},
		storageCfg,
	)

	require.NoError(t, err)
	host := storageHost{t: t, storageExtension: storageExtension}

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	storageFactory, err := jaegerstorage.GetStorageFactory(exporterCfg.TraceStorage, host)
	require.NoError(t, err)
	spanReader, err := storageFactory.CreateSpanReader()
	require.NoError(t, err)
	spanWriter, err := storageFactory.CreateSpanWriter()
	require.NoError(t, err)
	v.SpanReader = spanReader
	v.SpanWriter = spanWriter
	v.Refresh = s.refresh
	v.CleanUp = s.cleanUp
	v.TestGetLargeSpan(t)
}

func (s *StorageTest) refresh() error {
	return nil
}

func (s *StorageTest) cleanUp() error {
	return nil
}

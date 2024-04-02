// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/service/telemetry"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/datareceivers"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func (s *StorageIntegration) initTelemetry(t *testing.T, factories otelcol.Factories) (*jaegerstorage.Config, *storageexporter.Config, component.TelemetrySettings) {
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

	storageCfg, ok := cfg.Extensions[jaegerstorage.ID].(*jaegerstorage.Config)
	require.True(t, ok, "no jaeger storage extension found in the config")

	exporterCfg, ok := cfg.Exporters[storageexporter.ID].(*storageexporter.Config)
	require.True(t, ok, "no jaeger storage exporter found in the config")

	telemetrySettings := componenttest.NewNopTelemetrySettings()
	telemetrySettings.Logger = tel.Logger()

	return storageCfg, exporterCfg, telemetrySettings
}

func (s *StorageIntegration) Test(t *testing.T) {
	if os.Getenv("STORAGE") != s.Name {
		t.Skipf("Integration test against Jaeger-V2 %[1]s skipped; set STORAGE env var to %[1]s to run this", s.Name)
	}

	factories, err := internal.Components()
	require.NoError(t, err)

	dataProvider := testbed.NewGoldenDataProvider(
		"fixtures/generated_pict_pairs_traces.txt",
		"fixtures/generated_pict_pairs_spans.txt",
		"",
	)
	sender := testbed.NewOTLPTraceDataSender(testbed.DefaultHost, 4317)
	storageCfg, exporterCfg, telemetrySettings := s.initTelemetry(t, factories)

	// The data receiver will utilize the artificial jaeger receiver to pull
	// the traces from the database which is through jaeger storage extension.
	// Because of that, we need to host another jaeger storage extension
	// that is a duplication from the collector's extension. And get
	// the exporter TraceStorage name to set it to receiver TraceStorage.
	receiver := datareceivers.NewJaegerStorageDataReceiver(
		telemetrySettings,
		exporterCfg.TraceStorage,
		storageCfg,
	)

	runner := testbed.NewInProcessCollector(factories)
	validator := testbed.NewCorrectTestValidator(
		sender.ProtocolName(),
		receiver.ProtocolName(),
		dataProvider,
	)
	correctnessResults := &testbed.CorrectnessResults{}

	config, err := os.ReadFile(s.ConfigFile)
	require.NoError(t, err)
	t.Log(string(config))

	configCleanup, err := runner.PrepareConfig(string(config))
	require.NoError(t, err)
	defer configCleanup()

	tc := testbed.NewTestCase(
		t,
		dataProvider,
		sender,
		receiver,
		runner,
		validator,
		correctnessResults,
	)
	defer tc.Stop()

	tc.EnableRecording()
	tc.StartBackend()
	tc.StartAgent()

	tc.StartLoad(testbed.LoadOptions{
		DataItemsPerSecond: 1024,
		ItemsPerBatch:      1,
	})
	tc.Sleep(5 * time.Second)
	tc.StopLoad()

	tc.WaitForN(func() bool {
		return tc.LoadGenerator.DataItemsSent() == tc.MockBackend.DataItemsReceived()
	}, 10*time.Second, "all data items received")

	tc.StopBackend()

	tc.ValidateData()
}

func (s *StorageIntegration) UnitTest(t *testing.T) {
	if os.Getenv("STORAGE") != s.Name {
		t.Skipf("Integration test against Jaeger-V2 %[1]s skipped; set STORAGE env var to %[1]s to run this", s.Name)
	}
	factories, err := internal.Components()
	require.NoError(t, err)

	storageCfg, exporterCfg, telemetrySettings := s.initTelemetry(t, factories)

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

	s.storageExtension = storageExtension
	s.exporterCfg = exporterCfg

	s.tests = &integration.StorageIntegration{}

	s.initComponents(t)
	s.tests.RunAll(t)
}

func (s *StorageIntegration) initComponents(t *testing.T) {
	host := storageHost{t: t, storageExtension: s.storageExtension}
	err := s.storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.storageExtension.Shutdown(context.Background())) })
	storageFactory, err := jaegerstorage.GetStorageFactory(s.exporterCfg.TraceStorage, host)
	require.NoError(t, err)
	spanReader, err := storageFactory.CreateSpanReader()
	require.NoError(t, err)
	s.tests.SpanReader = spanReader
	spanWriter, err := storageFactory.CreateSpanWriter()
	require.NoError(t, err)
	s.tests.SpanWriter = spanWriter
	archiveSpanReader, err := storageFactory.CreateSpanReader()
	require.NoError(t, err)
	s.tests.ArchiveSpanReader = archiveSpanReader
	archiveSpanWriter, err := storageFactory.CreateSpanWriter()
	require.NoError(t, err)
	s.tests.ArchiveSpanWriter = archiveSpanWriter

	s.tests.Refresh = func(_ *testing.T) {}
	s.tests.CleanUp = s.cleanUp
}

func (s *StorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.storageExtension.Shutdown(context.Background()))

	time.Sleep(20 * time.Second)
	s.initComponents(t)
}

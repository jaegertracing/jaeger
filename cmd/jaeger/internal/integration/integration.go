// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/datareceivers"
)

type StorageIntegration struct {
	Name       string
	ConfigFile string
}

func (s *StorageIntegration) newDataReceiver(t *testing.T, factories otelcol.Factories) testbed.DataReceiver {
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

	config, ok := cfg.Extensions[jaegerstorage.ID].(*jaegerstorage.Config)
	require.True(t, ok, "no jaeger storage extension found in the config")

	// Hacky way to get the storage extension name.
	// This way we don't need to modify this,
	// when a new storage backend is added.
	name := ""
	vConfig := reflect.ValueOf(*config)
	for i := 0; i < vConfig.NumField(); i++ {
		keys := vConfig.Field(i).MapKeys()
		if len(keys) > 0 {
			name = keys[0].String()
			break
		}
	}
	require.NotEmpty(t, name, "failed to get jaeger storage extension name")

	receiver := datareceivers.NewJaegerStorageDataReceiver(name, config)
	return receiver
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
	receiver := s.newDataReceiver(t, factories)

	runner := testbed.NewInProcessCollector(factories)
	validator := testbed.NewCorrectTestValidator(
		sender.ProtocolName(),
		receiver.ProtocolName(),
		dataProvider,
	)
	correctnessResults := &testbed.CorrectnessResults{}

	config, err := os.ReadFile(s.ConfigFile)
	require.NoError(t, err)
	log.Println(string(config))

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

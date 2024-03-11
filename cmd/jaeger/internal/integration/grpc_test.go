// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/datareceivers"
	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
)

func TestGRPCStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "jaeger_grpc" {
		t.Skip("Integration test against Jaeger-V2 GRPC skipped; set STORAGE env var to jaeger_grpc to run this")
	}

	dataProvider := testbed.NewGoldenDataProvider(
		"fixtures/generated_pict_pairs_traces.txt",
		"fixtures/generated_pict_pairs_spans.txt",
		"",
	)
	sender := testbed.NewOTLPTraceDataSender(testbed.DefaultHost, 4317)
	receiver := datareceivers.NewJaegerStorageDataReceiver(
		"some-external-storage",
		&jaegerstorage.Config{
			GRPC: map[string]grpcCfg.Configuration{
				"some-external-storage": {
					RemoteServerAddr:     "127.0.0.1:17271",
					RemoteConnectTimeout: 5 * time.Second,
				},
			},
		},
	)

	factories, err := internal.Components()
	require.NoError(t, err)

	runner := testbed.NewInProcessCollector(factories)
	validator := testbed.NewCorrectTestValidator(
		sender.ProtocolName(),
		receiver.ProtocolName(),
		dataProvider,
	)
	correctnessResults := &testbed.CorrectnessResults{}

	config, err := os.ReadFile("fixtures/grpc_config.yaml")
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

// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/integration/datareceivers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

var correctnessResults testbed.TestResultsSummary = &testbed.CorrectnessResults{}

type KafkaIntegration struct {
	receiver   testbed.DataReceiver
	configFile string
}

func TestKafkaStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "otel_kafka" {
		t.Skip("Integration test against Jaeger v2 Kafka; set STORAGE env var to otel_kafka to run this")
	}

	dataProvider := testbed.NewGoldenDataProvider(
		"fixtures/generated_pict_pairs_traces.txt",
		"fixtures/generated_pict_pairs_spans.txt",
		"",
	)
	sender := testbed.NewOTLPTraceDataSender(testbed.DefaultHost, 4317)
	// LoadGenerator will be shared across testbed testcases
	// since we will validate the same origin data provided and the received traces
	loadGenerator, err := testbed.NewLoadGenerator(dataProvider, sender)
	require.NoError(t, err, "Cannot create generator")

	tests := []KafkaIntegration{
		{
			receiver:   datareceivers.NewKafkaDataReceiver(9092),
			configFile: "../collector-with-kafka.yaml",
		},
		{
			receiver:   datareceivers.NewJaegerStorageDataReceiver(17271),
			configFile: "../ingester-with-remote.yaml",
		},
	}

	for i, test := range tests {
		factories, err := internal.Components()
		require.NoError(t, err, "default components resulted in: %v", err)

		runner := testbed.NewInProcessCollector(factories)
		validator := testbed.NewCorrectTestValidator(sender.ProtocolName(), test.receiver.ProtocolName(), dataProvider)

		config, err := os.ReadFile(test.configFile)
		if err != nil {
			t.Fatal(err)
		}
		configCleanup, err := runner.PrepareConfig(string(config))
		require.NoError(t, err, "collector configuration resulted in: %v", err)
		defer configCleanup()

		tc := testbed.NewTestCase(
			t,
			dataProvider,
			sender,
			test.receiver,
			runner,
			validator,
			correctnessResults,
		)
		tc.LoadGenerator = loadGenerator
		defer tc.Stop()

		tc.EnableRecording()
		tc.StartBackend()
		tc.StartAgent()

		load := i == 0
		if load {
			tc.StartLoad(testbed.LoadOptions{
				DataItemsPerSecond: 16,
				ItemsPerBatch:      1,
			})
		}
		tc.Sleep(5 * time.Second)
		if load {
			tc.StopLoad()
		}

		tc.WaitForN(func() bool { return tc.LoadGenerator.DataItemsSent() == tc.MockBackend.DataItemsReceived() },
			10*time.Second, "all data items received")

		tc.StopBackend()

		tc.ValidateData()
	}
}

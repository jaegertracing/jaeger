// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

// KafkaCollectorIntegration handles the Jaeger collector with a custom configuration.
type KafkaCollectorIntegration struct {
	CollectorConfigFile string
	collectorCmd        *exec.Cmd
}

// e2eInitialize starts the Jaeger collector with the provided config file.
func (s *KafkaCollectorIntegration) e2eInitialize(t *testing.T) {
	collectorConfigFile := createStorageCleanerConfig(t, s.CollectorConfigFile, "kafka")

	// Start the collector
	t.Logf("Starting Jaeger collector in the background with config file %s", collectorConfigFile)
	s.startProcess(t, "collector", collectorConfigFile)
}

func (s *KafkaCollectorIntegration) startProcess(t *testing.T, processType, configFile string) {
	outFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), fmt.Sprintf("jaeger_%s_output_logs.txt", processType)),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the Jaeger %s output logs into %s", processType, outFile.Name())

	errFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), fmt.Sprintf("jaeger_%s_error_logs.txt", processType)),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the Jaeger %s error logs into %s", processType, errFile.Name())

	cmd := &exec.Cmd{
		Path: fmt.Sprintf("./cmd/jaeger/jaeger-%s", processType),
		Args: []string{fmt.Sprintf("jaeger-%s", processType), "--config", configFile},
		Dir:  "../../../..",
		Stdout: outFile,
		Stderr: errFile,
	}
	require.NoError(t, cmd.Start())

	require.Eventually(t, func() bool {
		url := fmt.Sprintf("http://localhost:%d/", ports.QueryHTTP)
		t.Logf("Checking if Jaeger %s is available on %s", processType, url)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Log(err)
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 500*time.Millisecond, fmt.Sprintf("Jaeger %s did not start", processType))
	t.Logf("Jaeger %s is ready", processType)

	if processType == "collector" {
		s.collectorCmd = cmd
	}
}

// e2eCleanUp stops the processes.
func (s *KafkaCollectorIntegration) e2eCleanUp(t *testing.T) {
	if s.collectorCmd != nil {
		require.NoError(t, s.collectorCmd.Process.Kill())
	}
}

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "kafka")

	// Initialize and start the collector
	collector := &KafkaCollectorIntegration{
		CollectorConfigFile: "../../collector-with-kafka.yaml",
	}
	collector.e2eInitialize(t)
	t.Cleanup(func() {
		collector.e2eCleanUp(t)
	})

	// Reuse E2EStorageIntegration for the ingester
	ingester := &E2EStorageIntegration{
		ConfigFile: "../../ingester-remote-storage.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,
		},
	}
	ingester.e2eInitialize(t, "kafka")
	t.Cleanup(func() {
		ingester.e2eCleanUp(t)
	})

	// Run the span store tests
	ingester.RunSpanStoreTests(t)
}

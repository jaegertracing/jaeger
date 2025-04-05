// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

const otlpPort = 4317

// E2EStorageIntegration holds components for e2e mode of Jaeger-v2
// storage integration test. The intended usage is as follows:
//   - Initialize a specific storage implementation declares its own test functions
//     (e.g. starts remote-storage).
//   - Then, instantiates with e2eInitialize() to run the Jaeger-v2 collector
//     and also the SpanWriter and SpanReader.
//   - After that, calls RunSpanStoreTests().
//   - Clean up with e2eCleanup() to close the SpanReader and SpanWriter connections.
//   - At last, clean up anything declared in its own test functions.
//     (e.g. close remote-storage)
type E2EStorageIntegration struct {
	integration.StorageIntegration

	SkipStorageCleaner bool
	ConfigFile         string
	BinaryName         string

	MetricsPort     int // overridable, default to 8888
	HealthCheckPort int // overridable for Kafka tests which run two binaries and need different ports

	// EnvVarOverrides contains a map of environment variables to set.
	// The key in the map is the environment variable to override and the value
	// is the value of the environment variable to set.
	// These variables are set upon initialization and are unset upon cleanup.
	EnvVarOverrides map[string]string
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
// This function should be called before any of the tests start.
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T, storage string) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	if s.BinaryName == "" {
		s.BinaryName = "jaeger-v2"
	}
	configFile := s.ConfigFile
	if !s.SkipStorageCleaner {
		configFile = createStorageCleanerConfig(t, s.ConfigFile, storage)
	}
	configFile, err := filepath.Abs(configFile)
	require.NoError(t, err, "Failed to get absolute path of the config file")
	require.FileExists(t, configFile, "Config file does not exist at the resolved path")

	t.Logf("Starting %s in the background with config file %s", s.BinaryName, configFile)
	cfgBytes, err := os.ReadFile(configFile)
	require.NoError(t, err)
	t.Logf("Config file content:\n%s", string(cfgBytes))

	envVars := []string{"OTEL_TRACES_SAMPLER=always_off"}
	for key, value := range s.EnvVarOverrides {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	cmd := Binary{
		Name:            s.BinaryName,
		HealthCheckPort: s.HealthCheckPort,
		Cmd: exec.Cmd{
			Path: "./cmd/jaeger/jaeger",
			Args: []string{"jaeger", "--config", configFile},
			// Change the working directory to the root of this project
			// since the binary config file jaeger_query's ui.config_file points to
			// "./cmd/jaeger/config-ui.json"
			Dir: "../../../..",
			Env: envVars,
		},
	}
	cmd.Start(t)

	s.TraceWriter, err = createTraceWriter(logger, otlpPort)
	require.NoError(t, err)

	s.TraceReader, err = createTraceReader(logger, ports.QueryGRPC)
	require.NoError(t, err)

	t.Cleanup(func() {
		s.scrapeMetrics(t, storage)
		require.NoError(t, s.TraceReader.(io.Closer).Close())
		require.NoError(t, s.TraceWriter.(io.Closer).Close())
	})
}

func (s *E2EStorageIntegration) scrapeMetrics(t *testing.T, storage string) {
	metricsPort := 8888
	if s.MetricsPort != 0 {
		metricsPort = s.MetricsPort
	}
	metricsUrl := fmt.Sprintf("http://localhost:%d/metrics", metricsPort)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, metricsUrl, nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	outputDir := "../../../../.metrics"
	require.NoError(t, os.MkdirAll(outputDir, os.ModePerm))

	metricsFile, err := os.Create(fmt.Sprintf("%s/metrics_snapshot_%v.txt", outputDir, storage))
	require.NoError(t, err)
	defer metricsFile.Close()

	_, err = io.Copy(metricsFile, resp.Body)
	require.NoError(t, err)
}

func createStorageCleanerConfig(t *testing.T, configFile string, storage string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	var config map[string]any
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	serviceAny, ok := config["service"]
	require.True(t, ok)
	service := serviceAny.(map[string]any)
	service["extensions"] = append(service["extensions"].([]any), "storage_cleaner")

	extensionsAny, ok := config["extensions"]
	require.True(t, ok)
	extensions := extensionsAny.(map[string]any)
	queryAny, ok := extensions["jaeger_query"]
	require.True(t, ok)
	storageAny, ok := queryAny.(map[string]any)["storage"]
	require.True(t, ok)
	traceStorageAny, ok := storageAny.(map[string]any)["traces"]
	require.True(t, ok)
	traceStorage := traceStorageAny.(string)
	extensions["storage_cleaner"] = map[string]string{"trace_storage": traceStorage}

	jaegerStorageAny, ok := extensions["jaeger_storage"]
	require.True(t, ok)
	jaegerStorage := jaegerStorageAny.(map[string]any)
	backendsAny, ok := jaegerStorage["backends"]
	require.True(t, ok)
	backends := backendsAny.(map[string]any)

	switch storage {
	case "elasticsearch", "opensearch":
		someStoreAny, ok := backends["some_storage"]
		require.True(t, ok, "expecting 'some_storage' entry, found: %v", jaegerStorage)
		someStore := someStoreAny.(map[string]any)
		esMainAny, ok := someStore[storage]
		require.True(t, ok, "expecting '%s' entry, found %v", storage, someStore)
		esMain := esMainAny.(map[string]any)
		esMain["service_cache_ttl"] = "1ms"
	default:
		// Do Nothing
	}

	newData, err := yaml.Marshal(config)
	require.NoError(t, err)
	fileExt := filepath.Ext(filepath.Base(configFile))
	fileName := strings.TrimSuffix(filepath.Base(configFile), fileExt)
	tempFile := filepath.Join(t.TempDir(), fileName+"_with_storageCleaner"+fileExt)
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err)

	t.Logf("Transformed configuration file %s to %s", configFile, tempFile)
	return tempFile
}

func purge(t *testing.T) {
	addr := fmt.Sprintf("http://0.0.0.0:%s%s", storagecleaner.Port, storagecleaner.URL)
	t.Logf("Purging storage via %s", addr)
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, addr, nil)
	require.NoError(t, err)

	client := &http.Client{}

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))
}

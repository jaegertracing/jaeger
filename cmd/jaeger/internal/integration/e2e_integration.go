// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
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

	SkipStorageCleaner  bool
	ConfigFile          string
	BinaryName          string
	HealthCheckEndpoint string

	// EnvVarOverrides contains a map of environment variables to set.
	// The key in the map is the environment variable to override and the value
	// is the value of the environment variable to set.
	// These variables are set upon initialization and are unset upon cleanup.
	EnvVarOverrides map[string]string
}

// Binary is a wrapper around exec.Cmd to help running binaries in tests.
type Binary struct {
	Name string
	exec.Cmd
}

func (b *Binary) Start(t *testing.T) {
	outFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "output_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the %s output logs into %s", b.Name, outFile.Name())

	errFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "error_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the %s error logs into %s", b.Name, errFile.Name())

	b.Stdout = outFile
	b.Stderr = errFile

	t.Logf("Running command: %v", b.Args)
	require.NoError(t, b.Cmd.Start())
	t.Cleanup(func() {
		if err := b.Process.Kill(); err != nil {
			t.Errorf("Failed to kill %s process: %v", b.Name, err)
		}
		if t.Failed() {
			// A special annotation to create a foldable section in the GitHub Actions runner output.
			// https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#grouping-log-lines
			fmt.Printf("::group::🚧 🚧 🚧 %s binary logs\n", b.Name)
			outLogs, err := os.ReadFile(outFile.Name())
			if err != nil {
				t.Errorf("Failed to read output logs: %v", err)
			} else {
				fmt.Printf("🚧 🚧 🚧 %s output logs:\n%s", b.Name, outLogs)
			}

			errLogs, err := os.ReadFile(errFile.Name())
			if err != nil {
				t.Errorf("Failed to read error logs: %v", err)
			} else {
				fmt.Printf("🚧 🚧 🚧 %s error logs:\n%s", b.Name, errLogs)
			}
			// End of Github Actions foldable section annotation.
			fmt.Println("::endgroup::")
		}
	})
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
// This function should be called before any of the tests start.
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T, storage string) {
	// set environment variable overrides
	for key, value := range s.EnvVarOverrides {
		t.Setenv(key, value)
	}
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
	cmd := Binary{
		Name: s.BinaryName,
		Cmd: exec.Cmd{
			Path: "./cmd/jaeger/jaeger",
			Args: []string{"jaeger", "--config", configFile},
			// Change the working directory to the root of this project
			// since the binary config file jaeger_query's ui.config_file points to
			// "./cmd/jaeger/config-ui.json"
			Dir: "../../../..",
		},
	}
	cmd.Start(t)

	// Wait for the binary to start and become ready to serve requests.
	require.Eventually(t, func() bool { return s.doHealthCheck(t) },
		60*time.Second, 3*time.Second, "%s did not start", s.BinaryName)
	t.Logf("%s is ready", s.BinaryName)

	s.SpanWriter, err = createSpanWriter(logger, otlpPort)
	require.NoError(t, err)
	s.SpanReader, err = createSpanReader(logger, ports.QueryGRPC)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Call e2eCleanUp to close the SpanReader and SpanWriter gRPC connection.
		s.e2eCleanUp(t)
	})
}

func (s *E2EStorageIntegration) doHealthCheck(t *testing.T) bool {
	healthCheckEndpoint := s.HealthCheckEndpoint
	if healthCheckEndpoint == "" {
		healthCheckEndpoint = "http://localhost:13133/status"
	}
	t.Logf("Checking if %s is available on %s", s.BinaryName, healthCheckEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthCheckEndpoint, nil)
	if err != nil {
		t.Logf("HTTP request creation failed: %v", err)
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("HTTP request failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Logf("Failed to read HTTP response body: %v", err)
		return false
	}
	if resp.StatusCode != http.StatusOK {
		t.Logf("HTTP response not OK: %v", string(body))
		return false
	}
	// for backwards compatibility with other healthchecks
	if !strings.HasSuffix(healthCheckEndpoint, "/status") {
		t.Logf("OK HTTP from endpoint that is not healthcheckv2")
		return true
	}

	var healthResponse struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&healthResponse); err != nil {
		t.Logf("Failed to decode JSON response '%s': %v", string(body), err)
		return false
	}

	// Check if the status field in the JSON is "StatusOK"
	if healthResponse.Status != "StatusOK" {
		t.Logf("Received non-K status %s: %s", healthResponse.Status, string(body))
		return false
	}
	return true
}

// e2eCleanUp closes the SpanReader and SpanWriter gRPC connection.
// This function should be called after all the tests are finished.
func (s *E2EStorageIntegration) e2eCleanUp(t *testing.T) {
	require.NoError(t, s.SpanReader.(io.Closer).Close())
	require.NoError(t, s.SpanWriter.(io.Closer).Close())
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
	tempFile := filepath.Join(t.TempDir(), "storageCleaner_config.yaml")
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

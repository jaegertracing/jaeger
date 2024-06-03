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
	ConfigFile string
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
// This function should be called before any of the tests start.
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T, storage string) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	configFile := createStorageCleanerConfig(t, s.ConfigFile, storage)
	t.Logf("Starting Jaeger-v2 in the background with config file %s", configFile)

	outFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "jaeger_output_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the Jaeger-v2 output logs into %s", outFile.Name())

	errFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "jaeger_error_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the Jaeger-v2 error logs into %s", errFile.Name())

	cmd := exec.Cmd{
		Path: "./cmd/jaeger/jaeger",
		Args: []string{"jaeger", "--config", configFile},
		// Change the working directory to the root of this project
		// since the binary config file jaeger_query's ui_config points to
		// "./cmd/jaeger/config-ui.json"
		Dir:    "../../../..",
		Stdout: outFile,
		Stderr: errFile,
	}
	require.NoError(t, cmd.Start())
	require.Eventually(t, func() bool {
		url := fmt.Sprintf("http://localhost:%d/", ports.QueryHTTP)
		t.Logf("Checking if Jaeger-v2 is available on %s", url)
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
	}, 30*time.Second, 500*time.Millisecond, "Jaeger-v2 did not start")
	t.Log("Jaeger-v2 is ready")
	t.Cleanup(func() {
		require.NoError(t, cmd.Process.Kill())
		if t.Failed() {
			// A Github Actions special annotation to create a foldable section
			// in the Github runner output.
			// https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#grouping-log-lines
			fmt.Println("::group::🚧 🚧 🚧 Jaeger-v2 binary logs")
			outLogs, err := os.ReadFile(outFile.Name())
			require.NoError(t, err)
			fmt.Printf("🚧 🚧 🚧 Jaeger-v2 output logs:\n%s", outLogs)

			errLogs, err := os.ReadFile(errFile.Name())
			require.NoError(t, err)
			fmt.Printf("🚧 🚧 🚧 Jaeger-v2 error logs:\n%s", errLogs)
			// End of Github Actions foldable section annotation.
			fmt.Println("::endgroup::")
		}
	})

	s.SpanWriter, err = createSpanWriter(logger, otlpPort)
	require.NoError(t, err)
	s.SpanReader, err = createSpanReader(logger, ports.QueryGRPC)
	require.NoError(t, err)
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

	service, ok := config["service"].(map[string]any)
	require.True(t, ok)
	service["extensions"] = append(service["extensions"].([]any), "storage_cleaner")

	extensions, ok := config["extensions"].(map[string]any)
	require.True(t, ok)
	query, ok := extensions["jaeger_query"].(map[string]any)
	require.True(t, ok)
	trace_storage := query["trace_storage"].(string)
	extensions["storage_cleaner"] = map[string]string{"trace_storage": trace_storage}

	jaegerStorage, ok := extensions["jaeger_storage"].(map[string]any)
	require.True(t, ok)

	switch storage {
	case "elasticsearch":
		elasticsearch, ok := jaegerStorage["elasticsearch"].(map[string]any)
		require.True(t, ok)
		esMain, ok := elasticsearch["es_main"].(map[string]any)
		require.True(t, ok)
		esMain["service_cache_ttl"] = "1ms"

	case "opensearch":
		opensearch, ok := jaegerStorage["opensearch"].(map[string]any)
		require.True(t, ok)
		osMain, ok := opensearch["os_main"].(map[string]any)
		require.True(t, ok)
		osMain["service_cache_ttl"] = "1ms"

	default:
		// Do Nothing
	}

	newData, err := yaml.Marshal(config)
	require.NoError(t, err)
	tempFile := filepath.Join(t.TempDir(), "storageCleaner_config.yaml")
	err = os.WriteFile(tempFile, newData, 0o600)
	require.NoError(t, err)

	return tempFile
}

func purge(t *testing.T) {
	addr := fmt.Sprintf("http://0.0.0.0:%s%s", storagecleaner.Port, storagecleaner.URL)
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, addr, nil)
	require.NoError(t, err)

	client := &http.Client{}

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

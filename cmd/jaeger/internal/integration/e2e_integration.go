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
	"go.yaml.in/yaml/v3"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

const otlpPort = 4317

// binaryCoverDirEnvVar names the directory where the spawned jaeger binary should
// write coverage counters. Deliberately not GOCOVERDIR — see binaryEnv.
const binaryCoverDirEnvVar = "JAEGER_BINARY_COVERDIR"

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
	BinaryPath         string // overrides default "./cmd/jaeger/jaeger"; resolved relative to the repo root

	MetricsPort     int // overridable, default to 8888
	HealthCheckPort int // overridable for tests (e.g. Kafka, query) which run two binaries and need different ports

	// EnvVarOverrides contains a map of environment variables to set.
	// The key in the map is the environment variable to override and the value
	// is the value of the environment variable to set.
	// These variables are set upon initialization and are unset upon cleanup.
	EnvVarOverrides map[string]string
	// PropagateEnvVars contains a list of environment variables to propagate
	// from the test process to the jaeger binary.
	PropagateEnvVars []string
	// FeatureGates contains a list of feature gate IDs to enable for the Jaeger binary.
	FeatureGates []string

	binary *Binary // set by e2eInitialize; allows mid-test shutdown via binary.Stop(t)
}

func (s *E2EStorageIntegration) args(configFile string) []string {
	args := []string{"jaeger", "--config", configFile}
	if len(s.FeatureGates) > 0 {
		args = append(args, "--feature-gates="+strings.Join(s.FeatureGates, ","))
	}
	return args
}

// binaryEnv builds the environment for the spawned jaeger binary. The child gets
// an explicit environment rather than inheriting the test process's, so anything
// it needs must be listed here.
//
// lookupEnv has os.LookupEnv's signature and is a parameter so that tests can
// exercise this without mutating the test process's own environment. That
// matters here: these tests run in the same process as the e2e tests, which rely
// on GOCOVERDIR being set, so a test that unset it would silently disable binary
// coverage for every test that ran afterwards.
func (s *E2EStorageIntegration) binaryEnv(lookupEnv func(string) (string, bool)) []string {
	envVars := []string{"OTEL_TRACES_SAMPLER=always_off"}
	// A binary built with `go build -cover` writes its coverage counters into
	// GOCOVERDIR when it exits. That directory cannot simply be inherited: when the
	// tests themselves run under `go test -coverprofile`, the toolchain sets
	// GOCOVERDIR in the test process to a temp dir of its own, so an inherited value
	// would be overwritten and the binary's counters would land somewhere the build
	// discards. The make target therefore passes the destination as
	// binaryCoverDirEnvVar and it is mapped onto GOCOVERDIR only for the child.
	if dir, ok := lookupEnv(binaryCoverDirEnvVar); ok && dir != "" {
		envVars = append(envVars, "GOCOVERDIR="+dir)
	}
	// Order preserved from before this function was extracted: os/exec keeps the
	// last of duplicate keys, so it decides which wins if a variable appears in
	// both EnvVarOverrides and PropagateEnvVars.
	for key, value := range s.EnvVarOverrides {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}
	for _, key := range s.PropagateEnvVars {
		if value, ok := lookupEnv(key); ok {
			envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
		}
	}
	return envVars
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

	envVars := s.binaryEnv(os.LookupEnv)

	binaryPath := s.BinaryPath
	if binaryPath == "" {
		binaryPath = "./cmd/jaeger/jaeger"
	}
	cmd := Binary{
		Name:            s.BinaryName,
		HealthCheckPort: s.HealthCheckPort,
		Cmd: exec.Cmd{
			Path: binaryPath,
			Args: s.args(configFile),
			// Change the working directory to the root of this project
			// since the binary config file jaeger_query's ui.config_file points to
			// "./cmd/jaeger/config-ui.json"
			Dir: "../../../..",
			Env: envVars,
		},
	}
	cmd.Start(t)
	s.binary = &cmd

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

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, metricsUrl, http.NoBody)
	require.NoError(t, err)

	client := testingHttpClient(t)
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
	service, ok := serviceAny.(map[string]any)
	require.True(t, ok, "expecting 'service' to be a map, found: %T", serviceAny)
	serviceExtensions, ok := service["extensions"].([]any)
	require.True(t, ok, "expecting 'service.extensions' to be a list, found: %T", service["extensions"])
	service["extensions"] = append(serviceExtensions, "storage_cleaner")

	extensionsAny, ok := config["extensions"]
	require.True(t, ok)
	extensions, ok := extensionsAny.(map[string]any)
	require.True(t, ok, "expecting 'extensions' to be a map, found: %T", extensionsAny)

	var traceStorage string

	// Try to get the storage from jaeger_query first
	if queryAny, found := extensions["jaeger_query"]; found {
		query, isMap := queryAny.(map[string]any)
		require.True(t, isMap, "expecting 'jaeger_query' to be a map, found: %T", queryAny)
		if storageAny, found := query["storage"]; found {
			storage, isMap := storageAny.(map[string]any)
			require.True(t, isMap, "expecting 'jaeger_query.storage' to be a map, found: %T", storageAny)
			if traceStorageAny, found := storage["traces"]; found {
				traceStorage, ok = traceStorageAny.(string)
				require.True(t, ok, "expecting 'jaeger_query.storage.traces' to be a string, found: %T", traceStorageAny)
			}
		}
	}

	// If jaeger_query not found or no storage, fallback to remote_storage
	if traceStorage == "" {
		if remoteAny, found := extensions["remote_storage"]; found {
			remote, isMap := remoteAny.(map[string]any)
			require.True(t, isMap, "expecting 'remote_storage' to be a map, found: %T", remoteAny)
			if storageNameAny, found := remote["storage"]; found {
				traceStorage, ok = storageNameAny.(string)
				require.True(t, ok, "expecting 'remote_storage.storage' to be a string, found: %T", storageNameAny)
			}
		}
	}

	require.NotEmpty(t, traceStorage, "traceStorage must be set from either jaeger_query or remote_storage")

	extensions["storage_cleaner"] = map[string]string{"trace_storage": traceStorage}

	jaegerStorageAny, ok := extensions["jaeger_storage"]
	require.True(t, ok)
	jaegerStorage, ok := jaegerStorageAny.(map[string]any)
	require.True(t, ok, "expecting 'jaeger_storage' to be a map, found: %T", jaegerStorageAny)
	backendsAny, ok := jaegerStorage["backends"]
	require.True(t, ok)
	backends, ok := backendsAny.(map[string]any)
	require.True(t, ok, "expecting 'backends' to be a map, found: %T", backendsAny)

	switch storage {
	case "elasticsearch", "opensearch":
		someStoreAny, ok := backends["some_storage"]
		require.True(t, ok, "expecting 'some_storage' entry, found: %v", jaegerStorage)
		someStore, ok := someStoreAny.(map[string]any)
		require.True(t, ok, "expecting 'some_storage' to be a map, found: %T", someStoreAny)
		esMainAny, ok := someStore[storage]
		require.True(t, ok, "expecting '%s' entry, found %v", storage, someStore)
		esMain, ok := esMainAny.(map[string]any)
		require.True(t, ok, "expecting '%s' to be a map, found: %T", storage, esMainAny)
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
	if purgerAddr, ok := os.LookupEnv("PURGER_ENDPOINT"); ok {
		addr = purgerAddr
	}
	t.Logf("Purging storage via %s", addr)
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, addr, http.NoBody)
	require.NoError(t, err)

	client := testingHttpClient(t)

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))
}

func testingHttpClient(t *testing.T) *http.Client {
	cl := http.DefaultClient
	t.Cleanup(func() {
		cl.CloseIdleConnections()
	})
	return cl
}

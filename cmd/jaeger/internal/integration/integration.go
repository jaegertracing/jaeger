// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/pkg/testutils"
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
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T) {
	logger, _ := testutils.NewLogger()
	s.ConfigFile = "./cmd/jaeger/internal/integration/" + createStorageCleanerConfig(t, s.ConfigFile)

	fmt.Println(s.ConfigFile)
	cmd := exec.Cmd{
		Path: "./cmd/jaeger/jaeger",
		Args: []string{"jaeger", "--config", s.ConfigFile},
		// Change the working directory to the root of this project
		// since the binary config file jaeger_query's ui_config points to
		// "./cmd/jaeger/config-ui.json"
		Dir:    "../../../..",
		Stdout: os.Stderr,
		Stderr: os.Stderr,
	}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		require.NoError(t, cmd.Process.Kill())
	})

	var err error
	s.SpanWriter, err = createSpanWriter(logger, otlpPort)
	require.NoError(t, err)
	s.SpanReader, err = createSpanReader(ports.QueryGRPC)
	require.NoError(t, err)
}

// e2eCleanUp closes the SpanReader and SpanWriter gRPC connection.
// This function should be called after all the tests are finished.
func (s *E2EStorageIntegration) e2eCleanUp(t *testing.T) {
	require.NoError(t, s.SpanReader.(io.Closer).Close())
	require.NoError(t, s.SpanWriter.(io.Closer).Close())
}

func createStorageCleanerConfig(t *testing.T, configFile string) string {
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	var config map[interface{}]interface{}
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	service, ok := config["service"].(map[interface{}]interface{})
	require.True(t, ok)
	service["extensions"] = []string{"jaeger_storage", "jaeger_query", "storage_cleaner"}

	extensions, ok := config["extensions"].(map[interface{}]interface{})
	require.True(t, ok)
	trace_storage := getTraceStorage(extensions)
	extensions["storage_cleaner"] = map[string]string{"trace_storage": trace_storage}

	newData, err := yaml.Marshal(config)
	require.NoError(t, err)
	tempFile, err := os.Create("storageCleaner_config.yaml")
	require.NoError(t, err)
	os.WriteFile(tempFile.Name(), newData, 0o644)

	t.Cleanup(func() {
		os.Remove(tempFile.Name())
	})
	return tempFile.Name()
}

func getTraceStorage(extensions map[interface{}]interface{}) string {
	for k := range extensions["jaeger_query"].(map[interface{}]interface{}) {
		if k == "trace_storage" {
			return extensions["jaeger_query"].(map[interface{}]interface{})[k].(string)
		}
	}
	return ""
}

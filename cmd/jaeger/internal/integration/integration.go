// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

const otlpPort = 4317

// E2EStorageIntegration holds components for e2e mode of Jaeger-v2
// storage integration test. The intended usage is as follows:
//   - A specific storage implementation declares its own test functions
//     (e.g. starts remote-storage).
//   - In those functions, instantiates with e2eInitialize()
//     and clean up with e2eCleanUp().
//   - Then calls RunTestSpanstore.
type E2EStorageIntegration struct {
	integration.StorageIntegration
	ConfigFile string
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T) {
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

	spanWriter := createSpanWriter(otlpPort)
	require.NoError(t, spanWriter.Start())
	s.SpanWriter = spanWriter

	spanReader := createSpanReader(ports.QueryGRPC)
	require.NoError(t, spanReader.Start())
	s.SpanReader = spanReader
}

func (s *E2EStorageIntegration) e2eCleanUp(t *testing.T) {
	require.NoError(t, s.SpanReader.(io.Closer).Close())
	require.NoError(t, s.SpanWriter.(io.Closer).Close())
}

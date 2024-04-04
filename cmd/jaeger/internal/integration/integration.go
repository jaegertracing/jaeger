// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

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

	binaryProcess *os.Process
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
	s.binaryProcess = cmd.Process

	var err error
	s.SpanWriter, err = createSpanWriter()
	require.NoError(t, err)
	s.SpanReader, err = createSpanReader()
	require.NoError(t, err)
}

func (s *E2EStorageIntegration) e2eCleanUp(_ *testing.T) {
	s.binaryProcess.Kill()
}

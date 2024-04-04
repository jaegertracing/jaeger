// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"strings"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
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

	runner        testbed.OtelcolRunner
	configCleanup func()
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
func (s *E2EStorageIntegration) e2eInitialize(t *testing.T) {
	factories, err := internal.Components()
	require.NoError(t, err)

	config, err := os.ReadFile(s.ConfigFile)
	require.NoError(t, err)
	config = []byte(strings.Replace(string(config), "./cmd/jaeger/", "../../", 1))

	s.runner = testbed.NewInProcessCollector(factories)
	s.configCleanup, err = s.runner.PrepareConfig(string(config))
	require.NoError(t, err)
	require.NoError(t, s.runner.Start(testbed.StartParams{}))

	s.SpanWriter, err = createSpanWriter()
	require.NoError(t, err)
	s.SpanReader, err = createSpanReader()
	require.NoError(t, err)
}

func (s *E2EStorageIntegration) e2eCleanUp(t *testing.T) {
	s.configCleanup()
	_, err := s.runner.Stop()
	require.NoError(t, err)
}

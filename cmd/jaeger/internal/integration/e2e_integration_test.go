// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigsAreValid(t *testing.T) {
	// Ensure that we can parse the existing configs correctly.
	// This is faster to run than the full integration test.
	validateConfig(t, "../../config-elasticsearch.yaml", "elasticsearch")
	validateConfig(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch")
	validateConfig(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch")
	validateConfig(t, "../../config-opensearch.yaml", "opensearch")
	validateConfig(t, "../../config-opensearch-manual-rollover.yaml", "opensearch")
	validateConfig(t, "../../config-opensearch-auto-rollover.yaml", "opensearch")
	validateConfig(t, "../../config-remote-storage-backend.yaml", "memory")
}

func validateConfig(t *testing.T, configFile string, storage string) {
	createStorageCleanerConfig(t, configFile, storage)
	removeBatchProcessor(t, configFile)
}

func fakeEnv(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestBinaryEnv(t *testing.T) {
	t.Run("cover dir is mapped onto GOCOVERDIR for the child", func(t *testing.T) {
		s := &E2EStorageIntegration{}
		env := s.binaryEnv(fakeEnv(map[string]string{binaryCoverDirEnvVar: "/tmp/coverdir"}))
		assert.Contains(t, env, "GOCOVERDIR=/tmp/coverdir")
	})

	t.Run("GOCOVERDIR is absent when the cover dir is unset", func(t *testing.T) {
		// A binary built without -cover ignores GOCOVERDIR, but passing an empty
		// value would make it write counters into the working directory.
		s := &E2EStorageIntegration{}
		for _, v := range s.binaryEnv(fakeEnv(nil)) {
			assert.NotContains(t, v, "GOCOVERDIR")
		}
	})

	t.Run("declared PropagateEnvVars still work", func(t *testing.T) {
		s := &E2EStorageIntegration{PropagateEnvVars: []string{"SOME_E2E_VAR", "NOT_SET_VAR"}}
		env := s.binaryEnv(fakeEnv(map[string]string{"SOME_E2E_VAR": "value"}))
		assert.Contains(t, env, "SOME_E2E_VAR=value")
		for _, v := range env {
			assert.NotContains(t, v, "NOT_SET_VAR")
		}
	})

	t.Run("an empty cover dir is not propagated", func(t *testing.T) {
		// An empty GOCOVERDIR would make the binary write counters into its working
		// directory rather than nowhere.
		s := &E2EStorageIntegration{}
		for _, v := range s.binaryEnv(fakeEnv(map[string]string{binaryCoverDirEnvVar: ""})) {
			assert.NotContains(t, v, "GOCOVERDIR")
		}
	})

	t.Run("overrides are included", func(t *testing.T) {
		s := &E2EStorageIntegration{EnvVarOverrides: map[string]string{"FOO": "bar"}}
		env := s.binaryEnv(fakeEnv(nil))
		assert.Contains(t, env, "FOO=bar")
		assert.Contains(t, env, "OTEL_TRACES_SAMPLER=always_off")
	})
}

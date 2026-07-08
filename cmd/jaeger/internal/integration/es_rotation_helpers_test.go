// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

const (
	esHostPort = "localhost:9200"
	esBaseURL  = "http://" + esHostPort
)

// runRotationSmokeTest is a helper that reduces boilerplate for rotation strategy
// e2e tests. It starts Jaeger with the given config and runs the smoke test battery.
// The setupFn is called after each purge to re-create rollover indices/aliases that
// purge deletes (purge does DELETE /*).
func runRotationSmokeTest(t *testing.T, configFile string, storage string, setupFn func(t *testing.T)) {
	s := &E2EStorageIntegration{
		ConfigFile: configFile,
		StorageIntegration: integration.StorageIntegration{
			CleanUp: func(t *testing.T) {
				purge(t)
				setupFn(t)
			},
			Capabilities: capabilities.ElasticsearchSmokeTest(),
		},
	}
	s.e2eInitialize(t, storage)
	s.RunSpanStoreTests(t)
}

// setupManualRolloverIndices uses the jaeger-es-rollover tool to create the
// initial indices and aliases for the manual_rollover strategy.
func setupManualRolloverIndices(t *testing.T, indexPrefix string) {
	initManualRolloverIndices(t, indexPrefix)
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, indexPrefix+"-")
	})
}

func initManualRolloverIndices(t *testing.T, indexPrefix string) {
	runEsRollover(t, "init", "--index-prefix="+indexPrefix)
}

// setupAutoRolloverIndices creates an ILM/ISM policy and then uses the
// jaeger-es-rollover tool to create initial indices and aliases for the
// auto_rollover strategy. openSearch selects an ISM vs ILM policy; the caller
// knows the backend statically (ES and OS have separate test files), so it is
// passed in rather than probed.
func setupAutoRolloverIndices(t *testing.T, indexPrefix, policyName string, openSearch bool) {
	initAutoRolloverIndices(t, indexPrefix, policyName, openSearch)
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, indexPrefix+"-")
		deleteLifecyclePolicy(t, policyName, openSearch)
	})
}

func initAutoRolloverIndices(t *testing.T, indexPrefix, policyName string, openSearch bool) {
	createLifecyclePolicy(t, policyName, openSearch)
	runEsRollover(
		t, "init",
		"--index-prefix="+indexPrefix,
		"--es.use-ilm=true",
		"--es.ilm-policy-name="+policyName,
	)
}

func runEsRollover(t *testing.T, action string, flags ...string) {
	args := append([]string{"run", "./cmd/es-rollover", action}, flags...)
	args = append(args, esBaseURL)
	cmd := exec.Command("go", args...)
	cmd.Dir = "../../../.."
	out, err := cmd.CombinedOutput()
	t.Logf("jaeger-es-rollover %s output:\n%s", action, string(out))
	require.NoError(t, err, "jaeger-es-rollover %s failed", action)
}

func createLifecyclePolicy(t *testing.T, policyName string, openSearch bool) {
	admin := newESAdmin(t)
	if openSearch {
		admin.putISMPolicy(policyName, `{
			"policy": {
				"description": "Jaeger e2e test policy",
				"default_state": "hot",
				"states": [{
					"name": "hot",
					"actions": [{"rollover": {"min_index_age": "1d"}}],
					"transitions": []
				}]
			}
		}`)
	} else {
		admin.putILMPolicy(policyName, `{"policy": {"phases": {"hot": {"min_age": "0ms", "actions": {"rollover": {"max_age": "1d"}}}}}}`)
	}
}

func deleteLifecyclePolicy(t *testing.T, policyName string, openSearch bool) {
	admin := newESAdmin(t)
	if openSearch {
		admin.deleteISMPolicy(policyName)
	} else {
		admin.deleteILMPolicy(policyName)
	}
}

func deleteIndicesByPrefix(t *testing.T, prefix string) {
	newESAdmin(t).deleteIndices(prefix + "*")
}

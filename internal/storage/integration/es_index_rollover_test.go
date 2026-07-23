// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	defaultILMPolicyName = "jaeger-ilm-policy"
)

func TestIndexRollover_FailIfILMNotPresent(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	client := createESClient(t)
	// make sure ES is clean
	cleanES(t, client, defaultILMPolicyName)
	envVars := []string{"ES_USE_ILM=true"}
	// Run the ES rollover test with adaptive sampling disabled (set to false).
	err := runEsRollover("init", envVars, false)
	require.EqualError(t, err, "exit status 1")
	indices := client.jaegerIndexNames(t, "")
	assert.Empty(t, indices)
}

func TestIndexRollover_Idempotency(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	client := createESClient(t)
	// Make sure that es is clean before the test!
	cleanES(t, client, defaultILMPolicyName)
	err := runEsRollover("init", []string{}, false)
	require.NoError(t, err)
	// Run again and it should return without any error
	err = runEsRollover("init", []string{}, false)
	require.NoError(t, err)
	cleanES(t, client, defaultILMPolicyName)
}

func TestIndexRollover_CreateIndicesWithILM(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	// Test using the default ILM Policy Name, i.e. do not pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run("DefaultPolicyName", func(t *testing.T) {
		runCreateIndicesWithILM(t, defaultILMPolicyName)
	})

	// Test using a configured ILM Policy Name, i.e. pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run("SetPolicyName", func(t *testing.T) {
		runCreateIndicesWithILM(t, "jaeger-test-policy")
	})
}

func runCreateIndicesWithILM(t *testing.T, ilmPolicyName string) {
	client := createESClient(t)
	version := client.backendVersion()

	envVars := []string{
		"ES_USE_ILM=true",
	}

	if ilmPolicyName != defaultILMPolicyName {
		envVars = append(envVars, "ES_ILM_POLICY_NAME="+ilmPolicyName)
	}

	expectedIndices := []string{"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001"}
	t.Run("NoPrefix", func(t *testing.T) {
		runIndexRolloverWithILMTest(t, client, version, "", expectedIndices, envVars, ilmPolicyName, false)
	})
	t.Run("WithPrefix", func(t *testing.T) {
		runIndexRolloverWithILMTest(t, client, version, indexPrefix, expectedIndices, append(envVars, "INDEX_PREFIX="+indexPrefix), ilmPolicyName, false)
	})
	t.Run("WithAdaptiveSampling", func(t *testing.T) {
		runIndexRolloverWithILMTest(t, client, version, indexPrefix, expectedIndices, append(envVars, "INDEX_PREFIX="+indexPrefix), ilmPolicyName, true)
	})
}

func runIndexRolloverWithILMTest(t *testing.T, client *esTestClient, version es.BackendVersion, prefix string, expectedIndices, envVars []string, ilmPolicyName string, adaptiveSampling bool) {
	writeAliases := []string{"jaeger-service-write", "jaeger-span-write", "jaeger-dependencies-write"}
	if adaptiveSampling {
		writeAliases = append(writeAliases, "jaeger-sampling-write")
		expectedIndices = append(expectedIndices, "jaeger-sampling-000001")
	}
	// make sure ES is cleaned before test
	cleanES(t, client, ilmPolicyName)
	// make sure ES is cleaned after test
	defer cleanES(t, client, ilmPolicyName)
	defer client.cleanTemplates(t, prefix)
	PutRolloverLifecyclePolicy(t, client.ilm, ilmPolicyName)

	if prefix != "" {
		prefix += "-"
	}
	var expected, expectedWriteAliases, actualWriteAliases []string
	for _, index := range expectedIndices {
		expected = append(expected, prefix+index)
	}
	for _, alias := range writeAliases {
		expectedWriteAliases = append(expectedWriteAliases, prefix+alias)
	}

	// Run rollover with given EnvVars
	err := runEsRollover("init", envVars, adaptiveSampling)
	require.NoError(t, err)

	indices := client.jaegerIndexNames(t, prefix)

	// Get settings and verify ILM policy name (ES) or ISM rollover alias (OpenSearch)
	settings := client.flatSettings(t, expected)
	for name, s := range settings {
		aliasKey := "index.lifecycle.rollover_alias"
		if version.IsOpenSearch() {
			aliasKey = "index.plugins.index_state_management.rollover_alias"
		} else {
			assert.Equal(t, ilmPolicyName, s["index.lifecycle.name"])
		}
		// Checked assertion: a missing/typeless key fails the test with a clear
		// message instead of panicking on the bare type assertion.
		alias, ok := s[aliasKey].(string)
		require.True(t, ok, "index %q settings missing string %q: %v", name, aliasKey, s)
		actualWriteAliases = append(actualWriteAliases, alias)
	}
	// Check indices created
	assert.ElementsMatch(t, indices, expected)
	// Check rollover alias is write alias
	assert.ElementsMatch(t, actualWriteAliases, expectedWriteAliases)
}

func cleanES(t *testing.T, client *esTestClient, policyName string) {
	client.deleteAllIndices(t)
	client.deleteLifecyclePolicy(t, policyName)
	client.cleanTemplates(t, "")
}

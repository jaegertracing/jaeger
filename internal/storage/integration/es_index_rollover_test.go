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
	assert.Empty(t, getJaegerIndices(t, client, ""))
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
	assert.ElementsMatch(t, getJaegerIndices(t, client, prefix), expected)
	// Check rollover alias is write alias
	assert.ElementsMatch(t, actualWriteAliases, expectedWriteAliases)
}

func getBackendVersion(client *elastic.Client) (es.BackendVersion, error) {
	pingResult, _, err := client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	parts := strings.SplitN(pingResult.Version.Number, ".", 2)
	majorVersion, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	return es.DetectBackendVersion(pingResult.TagLine, majorVersion), nil
}

// getJaegerIndices returns the names of the cluster's Jaeger indices for the
// given prefix. Unlike client.IndexNames(), which lists every index in the
// cluster, it queries the jaeger-* pattern directly, so unrelated system
// indices (e.g. OpenSearch's top_queries-*) never enter the result and the
// tests do not need to filter them out. See #7002.
func getJaegerIndices(t *testing.T, client *elastic.Client, prefix string) []string {
	settings, err := client.IndexGetSettings(prefix + "jaeger-*").Do(context.Background())
	require.NoError(t, err)
	indices := make([]string, 0, len(settings))
	for name := range settings {
		indices = append(indices, name)
	}
	return indices
}

func createILMPolicy(t *testing.T, client *elastic.Client, version es.BackendVersion, policyName string) {
	if version.IsOpenSearch() {
		createISMPolicy(t, policyName)
	} else {
		_, err := client.XPackIlmPutLifecycle().Policy(policyName).BodyString(`{"policy": {"phases": {"hot": {"min_age": "0ms","actions": {"rollover": {"max_age": "1d"},"set_priority": {"priority": 100}}}}}}`).Do(context.Background())
		require.NoError(t, err)
	}
}

func createISMPolicy(t *testing.T, policyName string) {
	policyBody := `{
		"policy": {
			"description": "Jaeger ILM integration test policy",
			"default_state": "hot",
			"states": [{
				"name": "hot",
				"actions": [{"rollover": {"min_index_age": "1d"}}],
				"transitions": []
			}]
		}
	}`
	url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", queryURL, policyName)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(policyBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := getESHttpClient(t).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK,
		"failed to create ISM policy (status %d): %s", resp.StatusCode, string(body))
}

func deleteISMPolicy(t *testing.T, policyName string) {
	url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", queryURL, policyName)
	req, err := http.NewRequest(http.MethodDelete, url, http.NoBody)
	require.NoError(t, err)
	resp, err := getESHttpClient(t).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	// 404 is expected if the policy doesn't exist yet
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		assert.Fail(t, "Not able to clean up ISM Policy", "status: %d", resp.StatusCode)
	}
}

func cleanES(t *testing.T, client *elastic.Client, policyName string) {
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	version, err := getBackendVersion(client)
	require.NoError(t, err)
	if version.IsOpenSearch() {
		deleteISMPolicy(t, policyName)
	} else {
		_, err = client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			assert.Fail(t, "Not able to clean up ILM Policy")
		}
	}
	_, err = client.IndexDeleteTemplate("*").Do(context.Background())
	require.NoError(t, err)
}

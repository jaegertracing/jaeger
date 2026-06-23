// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	defaultILMPolicyName = "jaeger-ilm-policy"
)

func TestIndexRollover_FailIfILMNotPresent(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client, err := createESClient(t, getESHttpClient(t))
	require.NoError(t, err)
	require.NoError(t, err)
	// make sure ES is clean
	cleanES(t, client, defaultILMPolicyName)
	envVars := []string{"ES_USE_ILM=true"}
	// Run the ES rollover test with adaptive sampling disabled (set to false).
	err = runEsRollover("init", envVars, false)
	require.EqualError(t, err, "exit status 1")
	indices, err := client.IndexNames()
	require.NoError(t, err)
	assert.Empty(t, indices)
}

func TestIndexRollover_Idempotency(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client, err := createESClient(t, getESHttpClient(t))
	require.NoError(t, err)
	// Make sure that es is clean before the test!
	cleanES(t, client, defaultILMPolicyName)
	err = runEsRollover("init", []string{}, false)
	require.NoError(t, err)
	// Run again and it should return without any error
	err = runEsRollover("init", []string{}, false)
	require.NoError(t, err)
	cleanES(t, client, defaultILMPolicyName)
}

func TestIndexRollover_CreateIndicesWithILM(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
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
	client, err := createESClient(t, getESHttpClient(t))
	require.NoError(t, err)
	version, err := getBackendVersion(client)
	require.NoError(t, err)

	if !version.SupportsILM() {
		t.Skipf("ILM/ISM not supported in %s", version)
	}

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

func runIndexRolloverWithILMTest(t *testing.T, client *elastic.Client, version es.BackendVersion, prefix string, expectedIndices, envVars []string, ilmPolicyName string, adaptiveSampling bool) {
	writeAliases := []string{"jaeger-service-write", "jaeger-span-write", "jaeger-dependencies-write"}
	if adaptiveSampling {
		writeAliases = append(writeAliases, "jaeger-sampling-write")
		expectedIndices = append(expectedIndices, "jaeger-sampling-000001")
	}
	// make sure ES is cleaned before test
	cleanES(t, client, ilmPolicyName)
	v8Client, err := createESV8Client(getESHttpClient(t).Transport)
	require.NoError(t, err)
	// make sure ES is cleaned after test
	defer cleanES(t, client, ilmPolicyName)
	defer cleanESIndexTemplates(t, client, v8Client, prefix)
	createILMPolicy(t, client, version, ilmPolicyName)

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
	err = runEsRollover("init", envVars, adaptiveSampling)
	require.NoError(t, err)

	indices, err := client.IndexNames()
	require.NoError(t, err)

	// Get settings and verify ILM/ISM policy is attached
	settings, err := client.IndexGetSettings(expected...).FlatSettings(true).Do(context.Background())
	require.NoError(t, err)
	for _, v := range settings {
		if version.IsOpenSearch() {
			actualWriteAliases = append(actualWriteAliases, v.Settings["index.plugins.index_state_management.rollover_alias"].(string))
		} else {
			assert.Equal(t, ilmPolicyName, v.Settings["index.lifecycle.name"])
			actualWriteAliases = append(actualWriteAliases, v.Settings["index.lifecycle.rollover_alias"].(string))
		}
	}
	// Check indices created
	assert.ElementsMatch(t, indices, expected)
	// Check rollover alias is write alias
	assert.ElementsMatch(t, actualWriteAliases, expectedWriteAliases)
}

func getBackendVersion(client *elastic.Client) (es.BackendVersion, error) {
	pingResult, _, err := client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	majorVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
	if err != nil {
		return 0, err
	}
	return es.DetectBackendVersion(pingResult.TagLine, majorVersion), nil
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
	require.Equal(t, http.StatusCreated, resp.StatusCode, "failed to create ISM policy: %s", string(body))
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
	} else if version.SupportsILM() {
		_, err = client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			assert.Fail(t, "Not able to clean up ILM Policy")
		}
	}
	_, err = client.IndexDeleteTemplate("*").Do(context.Background())
	require.NoError(t, err)
}

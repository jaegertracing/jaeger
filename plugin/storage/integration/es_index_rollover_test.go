// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build index_rollover
// +build index_rollover

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultILMPolicyName = "jaeger-ilm-policy"
	rolloverImage        = "jaegertracing/jaeger-es-rollover:latest"
)

func TestIndexRollover_FailIfILMNotPresent(t *testing.T) {
	client, err := createESClient()
	require.NoError(t, err)
	esVersion, err := getVersion(client)
	require.NoError(t, err)
	if esVersion != 7 {
		t.Skip("Integration test - " + t.Name() + " against ElasticSearch skipped for ES version " + fmt.Sprint(esVersion))
	}
	// make sure ES is clean
	cleanES(t, client, defaultILMPolicyName)
	envVars := []string{"ES_USE_ILM=true"}
	err = runEsRollover("init", envVars)
	assert.EqualError(t, err, "exit status 1")
	indices, err := client.IndexNames()
	require.NoError(t, err)
	assert.Empty(t, indices)
}

func TestIndexRollover_CreateIndicesWithILM(t *testing.T) {

	// Test using the default ILM Policy Name, i.e. do not pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run(fmt.Sprintf("DefaultPolicyName"), func(t *testing.T) {
		runCreateIndicesWithILM(t, defaultILMPolicyName)
	})

	// Test using a configured ILM Policy Name, i.e. pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run(fmt.Sprintf("SetPolicyName"), func(t *testing.T) {
		runCreateIndicesWithILM(t, "jaeger-test-policy")
	})
}

func runCreateIndicesWithILM(t *testing.T, ilmPolicyName string) {

	client, err := createESClient()
	require.NoError(t, err)

	esVersion, err := getVersion(client)
	require.NoError(t, err)

	envVars := []string{
		"ES_USE_ILM=true",
	}

	if ilmPolicyName != defaultILMPolicyName {
		envVars = append(envVars, "ES_ILM_POLICY_NAME="+ilmPolicyName)
	}

	if esVersion != 7 {
		cleanES(t, client, "")
		err := runEsRollover("init", envVars)
		assert.EqualError(t, err, "exit status 1")
		indices, err1 := client.IndexNames()
		require.NoError(t, err1)
		assert.Empty(t, indices)

	} else {
		expectedIndices := []string{"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001"}
		t.Run(fmt.Sprintf("NoPrefix"), func(t *testing.T) {
			runIndexRolloverWithILMTest(t, client, "", expectedIndices, envVars, ilmPolicyName)
		})
		t.Run(fmt.Sprintf("WithPrefix"), func(t *testing.T) {
			runIndexRolloverWithILMTest(t, client, indexPrefix, expectedIndices, append(envVars, "INDEX_PREFIX="+indexPrefix), ilmPolicyName)
		})
	}
}

func runIndexRolloverWithILMTest(t *testing.T, client *elastic.Client, prefix string, expectedIndices, envVars []string, ilmPolicyName string) {
	writeAliases := []string{"jaeger-service-write", "jaeger-span-write", "jaeger-dependencies-write"}

	// make sure ES is cleaned before test
	cleanES(t, client, ilmPolicyName)
	// make sure ES is cleaned after test
	defer cleanES(t, client, ilmPolicyName)
	err := createILMPolicy(client, ilmPolicyName)
	require.NoError(t, err)

	if prefix != "" {
		prefix = prefix + "-"
	}
	var expected, expectedWriteAliases, actualWriteAliases []string
	for _, index := range expectedIndices {
		expected = append(expected, prefix+index)
	}
	for _, alias := range writeAliases {
		expectedWriteAliases = append(expectedWriteAliases, prefix+alias)
	}

	// Run rollover with given EnvVars
	err = runEsRollover("init", envVars)
	require.NoError(t, err)

	indices, err := client.IndexNames()
	require.NoError(t, err)

	//Get ILM Policy Attached
	settings, err := client.IndexGetSettings(expected...).FlatSettings(true).Do(context.Background())
	require.NoError(t, err)
	//Check ILM Policy is attached and Get rollover alias attached
	for _, v := range settings {
		assert.Equal(t, ilmPolicyName, v.Settings["index.lifecycle.name"])
		actualWriteAliases = append(actualWriteAliases, v.Settings["index.lifecycle.rollover_alias"].(string))
	}
	//Check indices created
	assert.ElementsMatch(t, indices, expected, fmt.Sprintf("indices found: %v, expected: %v", indices, expected))
	//Check rollover alias is write alias
	assert.ElementsMatch(t, actualWriteAliases, expectedWriteAliases, fmt.Sprintf("aliases found: %v, expected: %v", actualWriteAliases, expectedWriteAliases))
}

func createESClient() (*elastic.Client, error) {
	return elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
}

func runEsRollover(action string, envs []string) error {
	var dockerEnv string
	for _, e := range envs {
		dockerEnv += fmt.Sprintf(" -e %s", e)
	}
	args := fmt.Sprintf("docker run %s --rm --net=host %s %s http://%s", dockerEnv, rolloverImage, action, queryHostPort)
	cmd := exec.Command("/bin/sh", "-c", args)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
}

func getVersion(client *elastic.Client) (uint, error) {
	pingResult, _, err := client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	esVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
	if err != nil {
		return 0, err
	}
	return uint(esVersion), nil
}

func createILMPolicy(client *elastic.Client, policyName string) error {
	_, err := client.XPackIlmPutLifecycle().Policy(policyName).BodyString(`{"policy": {"phases": {"hot": {"min_age": "0ms","actions": {"rollover": {"max_age": "1d"},"set_priority": {"priority": 100}}}}}}`).Do(context.Background())
	return err
}

func cleanES(t *testing.T, client *elastic.Client, policyName string) {
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	esVersion, err := getVersion(client)
	require.NoError(t, err)
	if esVersion == 7 {
		_, err = client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			assert.Fail(t, "Not able to clean up ILM Policy")
		}
	}
	_, err = client.IndexDeleteTemplate("*").Do(context.Background())
	require.NoError(t, err)
}

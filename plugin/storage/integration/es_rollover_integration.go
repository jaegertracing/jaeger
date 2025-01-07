// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"testing"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultILMPolicyName = "jaeger-ilm-policy"
)

type EsRolloverIntegration struct {
	runEsRollover func(action string, envs []string, adaptiveSampling bool) error
}

func NewESRolloverIntegration(runEsRollover func(action string, envs []string, adaptiveSampling bool) error) *EsRolloverIntegration {
	s := &EsRolloverIntegration{runEsRollover}
	return s
}

func newEsRolloverIntegration() *EsRolloverIntegration {
	return NewESRolloverIntegration(func(action string, envs []string, adaptiveSampling bool) error {
		var dockerEnv string
		for _, e := range envs {
			dockerEnv += " -e " + e
		}
		args := fmt.Sprintf("docker run %s --rm --net=host %s %s --adaptive-sampling=%t http://%s", dockerEnv, rolloverImage, action, adaptiveSampling, queryHostPort)
		cmd := exec.Command("/bin/sh", "-c", args)
		out, err := cmd.CombinedOutput()
		fmt.Println(string(out))
		return err
	})
}

func (s *EsRolloverIntegration) runCreateIndicesWithILM(t *testing.T, ilmPolicyName string) {
	client, err := createESClient(t, getESHttpClient(t))
	require.NoError(t, err)
	esVersion, err := getVersion(client)
	require.NoError(t, err)

	envVars := []string{
		"ES_USE_ILM=true",
	}

	if ilmPolicyName != defaultILMPolicyName {
		envVars = append(envVars, "ES_ILM_POLICY_NAME="+ilmPolicyName)
	}

	if esVersion >= 7 {
		expectedIndices := []string{"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001"}
		//t.Run("NoPrefix", func(t *testing.T) {
		//	s.runIndexRolloverWithILMTest(t, client, "", expectedIndices, envVars, ilmPolicyName, false)
		//})
		t.Run("WithPrefix", func(t *testing.T) {
			s.runIndexRolloverWithILMTest(t, client, indexPrefix, expectedIndices, append(envVars, "INDEX_PREFIX="+indexPrefix), ilmPolicyName, false)
		})
		t.Run("WithAdaptiveSampling", func(t *testing.T) {
			s.runIndexRolloverWithILMTest(t, client, indexPrefix, expectedIndices, append(envVars, "INDEX_PREFIX="+indexPrefix), ilmPolicyName, true)
		})
	}
}

func (s *EsRolloverIntegration) runIndexRolloverWithILMTest(t *testing.T, client *elastic.Client, prefix string, expectedIndices, envVars []string, ilmPolicyName string, adaptiveSampling bool) {
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
	err = createILMPolicy(client, ilmPolicyName)
	require.NoError(t, err)

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
	err = s.runEsRollover("init", envVars, adaptiveSampling)
	require.NoError(t, err)

	indices, err := client.IndexNames()
	require.NoError(t, err)

	// Get ILM Policy Attached
	settings, err := client.IndexGetSettings(expected...).FlatSettings(true).Do(context.Background())
	require.NoError(t, err)
	// Check ILM Policy is attached and Get rollover alias attached
	for _, v := range settings {
		assert.Equal(t, ilmPolicyName, v.Settings["index.lifecycle.name"])
		actualWriteAliases = append(actualWriteAliases, v.Settings["index.lifecycle.rollover_alias"].(string))
	}
	// Check indices created
	assert.ElementsMatch(t, indices, expected)
	// Check rollover alias is write alias
	assert.ElementsMatch(t, actualWriteAliases, expectedWriteAliases)
}

func (s *EsRolloverIntegration) testIndexRolloverFailIfILMNotPresent(t *testing.T) {
	client, err := createESClient(t, getESHttpClient(t))
	require.NoError(t, err)
	require.NoError(t, err)
	// make sure ES is clean
	cleanES(t, client, defaultILMPolicyName)
	envVars := []string{"ES_USE_ILM=true"}
	// Run the ES rollover test with adaptive sampling disabled (set to false).
	err = s.runEsRollover("init", envVars, false)
	require.EqualError(t, err, "exit status 1")
	indices, err := client.IndexNames()
	require.NoError(t, err)
	assert.Empty(t, indices)
}

func (s *EsRolloverIntegration) testIndexRolloverCreateIndicesWithILM(t *testing.T) {
	// Test using the default ILM Policy Name, i.e. do not pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run("DefaultPolicyName", func(t *testing.T) {
		s.runCreateIndicesWithILM(t, defaultILMPolicyName)
	})

	// Test using a configured ILM Policy Name, i.e. pass the ES_ILM_POLICY_NAME env var to the rollover script.
	t.Run("SetPolicyName", func(t *testing.T) {
		s.runCreateIndicesWithILM(t, "jaeger-test-policy")
	})
}

func (s *EsRolloverIntegration) RunESRolloverTests(t *testing.T, usingEsRolloverBinary bool) {
	if usingEsRolloverBinary {
		t.Run("FailIfILMNotPresent", s.testIndexRolloverFailIfILMNotPresent)
	}
	t.Run("CreateIndicesWithILM", s.testIndexRolloverCreateIndicesWithILM)
}

func createESClient(t *testing.T, hcl *http.Client) (*elastic.Client, error) {
	cl, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(hcl),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cl.Stop()
	})
	return cl, nil
}

func createESV8Client(tr http.RoundTripper) (*elasticsearch8.Client, error) {
	return elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
		Transport:            tr,
	})
}

func cleanESIndexTemplates(t *testing.T, client *elastic.Client, v8Client *elasticsearch8.Client, prefix string) {
	s := &ESStorageIntegration{
		client:   client,
		v8Client: v8Client,
	}
	s.cleanESIndexTemplates(t, prefix)
}

func getESHttpClient(t *testing.T) *http.Client {
	tr := &http.Transport{}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})
	return &http.Client{Transport: tr}
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
	if esVersion >= 7 {
		_, err = client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			assert.Fail(t, "Not able to clean up ILM Policy")
		}
	}
	_, err = client.IndexDeleteTemplate("*").Do(context.Background())
	require.NoError(t, err)
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
	//nolint: gosec
	return uint(esVersion), nil
}

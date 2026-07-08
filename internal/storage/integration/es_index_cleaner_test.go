// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	archiveIndexName      = "jaeger-span-archive"
	dependenciesIndexName = "jaeger-dependencies-2019-01-01"
	samplingIndexName     = "jaeger-sampling-2019-01-01"
	spanIndexName         = "jaeger-span-2019-01-01"
	serviceIndexName      = "jaeger-service-2019-01-01"
	indexCleanerImage     = "localhost:5000/jaegertracing/jaeger-es-index-cleaner:local-test"
	rolloverImage         = "localhost:5000/jaegertracing/jaeger-es-rollover:local-test"
	rolloverNowEnvVar     = `CONDITIONS='{"max_age":"0s"}'`
)

func TestIndexCleaner_doNotFailOnEmptyStorage(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client := createESClient(t)
	client.deleteIndices("*")

	tests := []struct {
		envs []string
	}{
		{envs: []string{"ROLLOVER=false"}},
		{envs: []string{"ROLLOVER=true"}},
		{envs: []string{"ARCHIVE=true"}},
	}
	for _, test := range tests {
		err := runEsCleaner(7, test.envs)
		require.NoError(t, err)
	}
}

func TestIndexCleaner_doNotFailOnFullStorage(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client := createESClient(t)
	tests := []struct {
		envs []string
	}{
		{envs: []string{"ROLLOVER=false"}},
		{envs: []string{"ROLLOVER=true"}},
		{envs: []string{"ARCHIVE=true"}},
	}
	for _, test := range tests {
		client.deleteIndices("*")
		// Create Indices with adaptive sampling disabled (set to false).
		err := createAllIndices(client, "", false)
		require.NoError(t, err)
		err = runEsCleaner(1500, test.envs)
		require.NoError(t, err)
	}
}

func TestIndexCleaner(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client := createESClient(t)

	tests := []struct {
		name             string
		envVars          []string
		expectedIndices  []string
		adaptiveSampling bool
	}{
		{
			name:    "RemoveDailyIndices",
			envVars: []string{},
			expectedIndices: []string{
				archiveIndexName,
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001", "jaeger-span-000002", "jaeger-service-000002", "jaeger-dependencies-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002",
			},
			adaptiveSampling: false,
		},
		{
			name:    "RemoveRolloverIndices",
			envVars: []string{"ROLLOVER=true"},
			expectedIndices: []string{
				archiveIndexName, spanIndexName, serviceIndexName, dependenciesIndexName, samplingIndexName,
				"jaeger-span-000002", "jaeger-service-000002", "jaeger-dependencies-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002",
			},
			adaptiveSampling: false,
		},
		{
			name:    "RemoveArchiveIndices",
			envVars: []string{"ARCHIVE=true"},
			expectedIndices: []string{
				archiveIndexName, spanIndexName, serviceIndexName, dependenciesIndexName, samplingIndexName,
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001", "jaeger-span-000002", "jaeger-service-000002", "jaeger-dependencies-000002",
				"jaeger-span-archive-000002",
			},
			adaptiveSampling: false,
		},
		{
			name:    "RemoveDailyIndices with adaptiveSampling",
			envVars: []string{},
			expectedIndices: []string{
				archiveIndexName,
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001", "jaeger-span-000002", "jaeger-service-000002", "jaeger-dependencies-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002", "jaeger-sampling-000001", "jaeger-sampling-000002",
			},
			adaptiveSampling: true,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_no_prefix, %s", test.name, test.envVars), func(t *testing.T) {
			runIndexCleanerTest(t, client, "", test.expectedIndices, test.envVars, test.adaptiveSampling)
		})
		t.Run(fmt.Sprintf("%s_prefix, %s", test.name, test.envVars), func(t *testing.T) {
			runIndexCleanerTest(t, client, indexPrefix, test.expectedIndices, append(test.envVars, "INDEX_PREFIX="+indexPrefix), test.adaptiveSampling)
		})
	}
}

func runIndexCleanerTest(t *testing.T, client *esTestClient, prefix string, expectedIndices, envVars []string, adaptiveSampling bool) {
	// make sure ES is clean
	client.deleteIndices("*")
	defer client.cleanTemplates(prefix)
	err := createAllIndices(client, prefix, adaptiveSampling)
	require.NoError(t, err)
	err = runEsCleaner(0, envVars)
	require.NoError(t, err)
	foundIndices := client.indexNames()
	if prefix != "" {
		prefix += "-"
	}
	var actual []string
	for _, index := range foundIndices {
		// ignore system indices https://github.com/jaegertracing/jaeger/issues/7002
		if strings.HasPrefix(index, prefix+"jaeger") {
			actual = append(actual, index)
		}
	}
	var expected []string
	for _, index := range expectedIndices {
		expected = append(expected, prefix+index)
	}
	assert.ElementsMatch(t, actual, expected, "indices found: %v, expected: %v", foundIndices, expected)
}

func createAllIndices(client *esTestClient, prefix string, adaptiveSampling bool) error {
	prefixWithSeparator := prefix
	if prefix != "" {
		prefixWithSeparator += "-"
	}
	// create daily indices and archive index
	createEsIndices(client, []string{
		prefixWithSeparator + spanIndexName,
		prefixWithSeparator + serviceIndexName,
		prefixWithSeparator + dependenciesIndexName,
		prefixWithSeparator + samplingIndexName,
		prefixWithSeparator + archiveIndexName,
	})
	// create rollover archive index and roll alias to the new index
	err := runEsRollover("init", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix}, adaptiveSampling)
	if err != nil {
		return err
	}
	err = runEsRollover("rollover", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling)
	if err != nil {
		return err
	}
	// create rollover main indices and roll over to the new index
	err = runEsRollover("init", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix}, adaptiveSampling)
	if err != nil {
		return err
	}
	err = runEsRollover("rollover", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling)
	if err != nil {
		return err
	}
	return nil
}

func createEsIndices(client *esTestClient, indices []string) {
	for _, index := range indices {
		client.createIndex(index)
	}
}

func runEsCleaner(days int, envs []string) error {
	var dockerEnv strings.Builder
	for _, e := range envs {
		dockerEnv.WriteString(" -e ")
		dockerEnv.WriteString(e)
	}
	args := fmt.Sprintf("docker run %s --rm --net=host %s %d http://%s", dockerEnv.String(), indexCleanerImage, days, queryHostPort)
	cmd := exec.Command("/bin/sh", "-c", args)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
}

func runEsRollover(action string, envs []string, adaptiveSampling bool) error {
	var dockerEnv strings.Builder
	for _, e := range envs {
		dockerEnv.WriteString(" -e ")
		dockerEnv.WriteString(e)
	}
	args := fmt.Sprintf("docker run %s --rm --net=host %s %s --adaptive-sampling=%t http://%s", dockerEnv.String(), rolloverImage, action, adaptiveSampling, queryHostPort)
	cmd := exec.Command("/bin/sh", "-c", args)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
}

func createESClient(t *testing.T) *esTestClient {
	return newESTestClient(t)
}

func getESHttpClient(t *testing.T) *http.Client {
	tr := &http.Transport{}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})
	return &http.Client{Transport: tr}
}

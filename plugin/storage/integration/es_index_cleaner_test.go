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

// +build index_cleaner

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	archiveIndexName      = "jaeger-span-archive"
	dependenciesIndexName = "jaeger-dependencies-2019-01-01"
	spanIndexName         = "jaeger-span-2019-01-01"
	serviceIndexName      = "jaeger-service-2019-01-01"
	indexCleanerImage     = "jaegertracing/jaeger-es-index-cleaner:latest"
	rolloverImage         = "jaegertracing/jaeger-es-rollover:latest"
	rolloverNowEnvVar     = "CONDITIONS='{\"max_age\":\"0s\"}'"
)

func TestIndexCleaner_doNotFailOnEmptyStorage(t *testing.T) {
	client, err := createESClient()
	require.NoError(t, err)
	_, err = client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)

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
	client, err := createESClient()
	require.NoError(t, err)
	tests := []struct {
		envs []string
	}{
		{envs: []string{"ROLLOVER=false"}},
		{envs: []string{"ROLLOVER=true"}},
		{envs: []string{"ARCHIVE=true"}},
	}
	for _, test := range tests {
		_, err = client.DeleteIndex("*").Do(context.Background())
		require.NoError(t, err)
		err := createAllIndices(client, "")
		require.NoError(t, err)
		err = runEsCleaner(1500, test.envs)
		require.NoError(t, err)
	}
}

func TestIndexCleaner(t *testing.T) {
	client, err := createESClient()
	require.NoError(t, err)

	tests := []struct {
		name            string
		envVars         []string
		expectedIndices []string
	}{
		{
			name:    "RemoveDailyIndices",
			envVars: []string{},
			expectedIndices: []string{
				archiveIndexName,
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-span-000002", "jaeger-service-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002",
			},
		},
		{
			name:    "RemoveRolloverIndices",
			envVars: []string{"ROLLOVER=true"},
			expectedIndices: []string{
				archiveIndexName, spanIndexName, serviceIndexName, dependenciesIndexName,
				"jaeger-span-000002", "jaeger-service-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002",
			},
		},
		{
			name:    "RemoveArchiveIndices",
			envVars: []string{"ARCHIVE=true"},
			expectedIndices: []string{
				archiveIndexName, spanIndexName, serviceIndexName, dependenciesIndexName,
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-span-000002", "jaeger-service-000002",
				"jaeger-span-archive-000002",
			},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_no_prefix", test.name), func(t *testing.T) {
			runIndexCleanerTest(t, client, "", test.expectedIndices, test.envVars)
		})
		t.Run(fmt.Sprintf("%s_prefix", test.name), func(t *testing.T) {
			runIndexCleanerTest(t, client, indexPrefix, test.expectedIndices, append(test.envVars, "INDEX_PREFIX="+indexPrefix))
		})
	}
}

func runIndexCleanerTest(t *testing.T, client *elastic.Client, prefix string, expectedIndices, envVars []string) {
	// make sure ES is clean
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)

	err = createAllIndices(client, prefix)
	require.NoError(t, err)
	err = runEsCleaner(0, envVars)
	require.NoError(t, err)

	indices, err := client.IndexNames()
	require.NoError(t, err)
	if prefix != "" {
		prefix = prefix + "-"
	}
	var expected []string
	for _, index := range expectedIndices {
		expected = append(expected, prefix+index)
	}
	assert.ElementsMatch(t, indices, expected, fmt.Sprintf("indices found: %v, expected: %v", indices, expected))
}

func createAllIndices(client *elastic.Client, prefix string) error {
	prefixWithSeparator := prefix
	if prefix != "" {
		prefixWithSeparator = prefixWithSeparator + "-"
	}
	// create daily indices and archive index
	err := createEsIndices(client, []string{
		prefixWithSeparator + spanIndexName, prefixWithSeparator + serviceIndexName,
		prefixWithSeparator + dependenciesIndexName, prefixWithSeparator + archiveIndexName})
	if err != nil {
		return err
	}
	// create rollover archive index and roll alias to the new index
	err = runEsRollover("init", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix})
	if err != nil {
		return err
	}
	err = runEsRollover("rollover", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar})
	if err != nil {
		return err
	}
	// create rollover main indices and roll over to the new index
	err = runEsRollover("init", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix})
	if err != nil {
		return err
	}
	err = runEsRollover("rollover", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar})
	if err != nil {
		return err
	}
	return nil
}

func createEsIndices(client *elastic.Client, indices []string) error {
	for _, index := range indices {
		if _, err := client.CreateIndex(index).Do(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

func runEsCleaner(days int, envs []string) error {
	var dockerEnv string
	for _, e := range envs {
		dockerEnv += fmt.Sprintf(" -e %s", e)
	}
	args := fmt.Sprintf("docker run %s --net=host %s %d http://%s", dockerEnv, indexCleanerImage, days, queryHostPort)
	cmd := exec.Command("/bin/sh", "-c", args)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
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

func createESClient() (*elastic.Client, error) {
	return elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
}

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

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	archiveIndexName      = "jaeger-span-archive"
	dependenciesIndexName = "jaeger-dependencies-2019-01-01"
	samplingIndexName     = "jaeger-sampling-2019-01-01"
	spanIndexName         = "jaeger-span-2019-01-01"
	serviceIndexName      = "jaeger-service-2019-01-01"
	indexCleanerImage     = "jaegertracing/jaeger-es-index-cleaner:local-test"
	rolloverImage         = "jaegertracing/jaeger-es-rollover:latest"
	rolloverNowEnvVar     = `CONDITIONS='{"max_age":"0s"}'`
)

func TestIndexCleaner_doNotFailOnEmptyStorage(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
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
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
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
		// Create Indices with adaptive sampling disabled (set to false).
		err := createAllIndices(client, "", false)
		require.NoError(t, err)
		err = runEsCleaner(1500, test.envs)
		require.NoError(t, err)
	}
}

func TestIndexCleaner(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	client, err := createESClient()
	require.NoError(t, err)
	v8Client, err := createESV8Client()
	require.NoError(t, err)

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
			runIndexCleanerTest(t, client, v8Client, "", test.expectedIndices, test.envVars, test.adaptiveSampling)
		})
		t.Run(fmt.Sprintf("%s_prefix, %s", test.name, test.envVars), func(t *testing.T) {
			runIndexCleanerTest(t, client, v8Client, indexPrefix, test.expectedIndices, append(test.envVars, "INDEX_PREFIX="+indexPrefix), test.adaptiveSampling)
		})
	}
}

func runIndexCleanerTest(t *testing.T, client *elastic.Client, v8Client *elasticsearch8.Client, prefix string, expectedIndices, envVars []string, adaptiveSampling bool) {
	// make sure ES is clean
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	defer cleanESIndexTemplates(t, client, v8Client, prefix)
	err = createAllIndices(client, prefix, adaptiveSampling)
	require.NoError(t, err)
	err = runEsCleaner(0, envVars)
	require.NoError(t, err)
	indices, err := client.IndexNames()
	require.NoError(t, err)
	if prefix != "" {
		prefix += "-"
	}
	var expected []string
	for _, index := range expectedIndices {
		expected = append(expected, prefix+index)
	}
	assert.ElementsMatch(t, indices, expected, fmt.Sprintf("indices found: %v, expected: %v", indices, expected))
}

func createAllIndices(client *elastic.Client, prefix string, adaptiveSampling bool) error {
	prefixWithSeparator := prefix
	if prefix != "" {
		prefixWithSeparator += "-"
	}
	// create daily indices and archive index
	err := createEsIndices(client, []string{
		prefixWithSeparator + spanIndexName,
		prefixWithSeparator + serviceIndexName,
		prefixWithSeparator + dependenciesIndexName,
		prefixWithSeparator + samplingIndexName,
		prefixWithSeparator + archiveIndexName,
	})
	if err != nil {
		return err
	}
	// create rollover archive index and roll alias to the new index
	err = runEsRollover("init", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix}, adaptiveSampling)
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
	args := fmt.Sprintf("docker run %s --rm --net=host %s %d http://%s", dockerEnv, indexCleanerImage, days, queryHostPort)
	cmd := exec.Command("/bin/sh", "-c", args)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
}

func runEsRollover(action string, envs []string, adaptiveSampling bool) error {
	var dockerEnv string
	for _, e := range envs {
		dockerEnv += fmt.Sprintf(" -e %s", e)
	}
	args := fmt.Sprintf("docker run %s --rm --net=host %s %s --adaptive-sampling=%t http://%s", dockerEnv, rolloverImage, action, adaptiveSampling, queryHostPort)
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

func createESV8Client() (*elasticsearch8.Client, error) {
	return elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
	})
}

func cleanESIndexTemplates(t *testing.T, client *elastic.Client, v8Client *elasticsearch8.Client, prefix string) {
	s := &ESStorageIntegration{
		client:   client,
		v8Client: v8Client,
	}
	s.cleanESIndexTemplates(t, prefix)
}

// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
	indexCleanerImage     = "localhost:5000/jaegertracing/jaeger-es-index-cleaner:local-test"
	rolloverImage         = "localhost:5000/jaegertracing/jaeger-es-rollover:local-test"
	rolloverNowEnvVar     = `CONDITIONS='{"max_age":"0s"}'`
)

type EsCleanerIntegration struct {
	runEsRollover func(action string, envs []string, adaptiveSampling bool) error
	runEsCleaner  func(days int, envs []string) error
}

func NewEsCleanerIntegration(runEsRollover func(action string, envs []string, adaptiveSampling bool) error, runEsCleaner func(days int, envs []string) error) *EsCleanerIntegration {
	return &EsCleanerIntegration{
		runEsRollover: runEsRollover,
		runEsCleaner:  runEsCleaner,
	}
}

func newEsCleanerIntegration() *EsCleanerIntegration {
	rollover := newEsRolloverIntegration()
	return NewEsCleanerIntegration(rollover.runEsRollover, func(days int, envs []string) error {
		var dockerEnv string
		for _, e := range envs {
			dockerEnv += " -e " + e
		}
		args := fmt.Sprintf("docker run %s --rm --net=host %s %d http://%s", dockerEnv, indexCleanerImage, days, queryHostPort)
		cmd := exec.Command("/bin/sh", "-c", args)
		out, err := cmd.CombinedOutput()
		fmt.Println(string(out))
		return err
	})
}

func (s *EsCleanerIntegration) createAllIndices(client *elastic.Client, prefix string, adaptiveSampling bool) error {
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
	err = s.runEsRollover("init", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix}, adaptiveSampling)
	if err != nil {
		return err
	}
	err = s.runEsRollover("rollover", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling)
	if err != nil {
		return err
	}
	// create rollover main indices and roll over to the new index
	err = s.runEsRollover("init", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix}, adaptiveSampling)
	if err != nil {
		return err
	}
	err = s.runEsRollover("rollover", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling)
	if err != nil {
		return err
	}
	return nil
}

func (s *EsCleanerIntegration) runIndexCleanerTest(t *testing.T, client *elastic.Client, v8Client *elasticsearch8.Client, prefix string, expectedIndices, envVars []string, adaptiveSampling bool) {
	// make sure ES is clean
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	defer cleanESIndexTemplates(t, client, v8Client, prefix)
	err = s.createAllIndices(client, prefix, adaptiveSampling)
	require.NoError(t, err)
	err = s.runEsCleaner(0, envVars)
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
	assert.ElementsMatch(t, indices, expected, "indices found: %v, expected: %v", indices, expected)
}

func (s *EsCleanerIntegration) esCleanerTests(t *testing.T) {
	hcl := getESHttpClient(t)
	client, err := createESClient(t, hcl)
	require.NoError(t, err)
	v8Client, err := createESV8Client(hcl.Transport)
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
			s.runIndexCleanerTest(t, client, v8Client, "", test.expectedIndices, test.envVars, test.adaptiveSampling)
		})
		t.Run(fmt.Sprintf("%s_prefix, %s", test.name, test.envVars), func(t *testing.T) {
			s.runIndexCleanerTest(t, client, v8Client, indexPrefix, test.expectedIndices, append(test.envVars, "INDEX_PREFIX="+indexPrefix), test.adaptiveSampling)
		})
	}
}

func (s *EsCleanerIntegration) testIndexCleanerDoNotFailOnFullStorage(t *testing.T) {
	client, err := createESClient(t, getESHttpClient(t))
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
		err := s.createAllIndices(client, "", false)
		require.NoError(t, err)
		err = s.runEsCleaner(1500, test.envs)
		require.NoError(t, err)
	}
}

func (s *EsCleanerIntegration) testIndexCleanerDoNotFailOnEmptyStorage(t *testing.T) {
	client, err := createESClient(t, getESHttpClient(t))
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
		err := s.runEsCleaner(7, test.envs)
		require.NoError(t, err)
	}
}

func (s *EsCleanerIntegration) RunEsCleanerTests(t *testing.T) {
	t.Run("IndexCleaner", s.esCleanerTests)
	t.Run("DoNotFailOnFullStorage", s.testIndexCleanerDoNotFailOnFullStorage)
	t.Run("DoNotFailOnEmptyStorage", s.testIndexCleanerDoNotFailOnEmptyStorage)
}

func createEsIndices(client *elastic.Client, indices []string) error {
	for _, index := range indices {
		if _, err := client.CreateIndex(index).Do(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
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
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client, err := createESClient(getESHttpClient(t))
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
		require.NoError(t, runEsCleaner(7, test.envs))
	}
}

func TestIndexCleaner_doNotFailOnFullStorage(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	client, err := createESClient(getESHttpClient(t))
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
		require.NoError(t, createAllIndices(client, "", false))
		require.NoError(t, runEsCleaner(1500, test.envs))
	}
}

func TestIndexCleaner(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	hcl := getESHttpClient(t)
	client, err := createESClient(hcl)
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
				"jaeger-span-000001", "jaeger-service-000001", "jaeger-dependencies-000001",
				"jaeger-span-000002", "jaeger-service-000002", "jaeger-dependencies-000002",
				"jaeger-span-archive-000001", "jaeger-span-archive-000002",
			},
			adaptiveSampling: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runIndexCleanerTest(
				t,
				client,
				v8Client,
				"",
				test.expectedIndices,
				test.envVars,
				test.adaptiveSampling,
			)
		})
	}
}

func runIndexCleanerTest(
	t *testing.T,
	client *elastic.Client,
	v8Client *elasticsearch8.Client,
	prefix string,
	expectedIndices, envVars []string,
	adaptiveSampling bool,
) {
	// make sure ES is clean
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cleanESIndexTemplates(client, v8Client, prefix))
	})

	require.NoError(t, createAllIndices(client, prefix, adaptiveSampling))
	require.NoError(t, runEsCleaner(0, envVars))

	foundIndices, err := client.IndexNames()
	require.NoError(t, err)
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
	assert.ElementsMatch(t, actual, expected)
}

func createAllIndices(client *elastic.Client, prefix string, adaptiveSampling bool) error {
	p := prefix
	if p != "" {
		p += "-"
	}
	// create daily indices and archive index
	if err := createEsIndices(client, []string{
		p + spanIndexName,
		p + serviceIndexName,
		p + dependenciesIndexName,
		p + samplingIndexName,
		p + archiveIndexName,
	}); err != nil {
		return err
	}
	// create rollover archive index and roll alias to the new index
	if err := runEsRollover("init", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix}, adaptiveSampling); err != nil {
		return err
	}
	if err := runEsRollover("rollover", []string{"ARCHIVE=true", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling); err != nil {
		return err
	}
	// create rollover main indices and roll over to the new index
	if err := runEsRollover("init", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix}, adaptiveSampling); err != nil {
		return err
	}
	return runEsRollover("rollover", []string{"ARCHIVE=false", "INDEX_PREFIX=" + prefix, rolloverNowEnvVar}, adaptiveSampling)
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
	var dockerEnv strings.Builder
	for _, e := range envs {
		dockerEnv.WriteString(" -e ")
		dockerEnv.WriteString(e)
	}
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		fmt.Sprintf("docker run %s --rm --net=host %s %d http://%s",
			dockerEnv.String(),
			indexCleanerImage,
			days,
			queryHostPort,
		),
	)
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
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		fmt.Sprintf(
			"docker run %s --rm --net=host %s %s --adaptive-sampling=%t http://%s",
			dockerEnv.String(),
			rolloverImage,
			action,
			adaptiveSampling,
			queryHostPort,
		),
	)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	return err
}

func createESClient(hcl *http.Client) (*elastic.Client, error) {
	cl, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(hcl),
	)
	if err != nil {
		return nil, err
	}
	return cl, nil
}

func createESV8Client(tr http.RoundTripper) (*elasticsearch8.Client, error) {
	return elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
		Transport:            tr,
	})
}

func cleanESIndexTemplates(client *elastic.Client, v8Client *elasticsearch8.Client, prefix string) error {
	s := &ESStorageIntegration{
		client:   client,
		v8Client: v8Client,
	}
	return s.cleanESIndexTemplates(prefix)
}

func getESHttpClient(t *testing.T) *http.Client {
	tr := &http.Transport{}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})
	return &http.Client{Transport: tr}
}

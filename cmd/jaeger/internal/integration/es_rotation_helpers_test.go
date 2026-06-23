// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/require"
)

const esURL = "http://localhost:9200"

// setupManualRolloverIndices creates the initial indices and aliases that the
// manual_rollover strategy expects to find. It mirrors what `jaeger-es-rollover init`
// does, but without requiring the Docker image.
func setupManualRolloverIndices(t *testing.T, indexPrefix string) {
	client := newESClient(t)
	prefix := indexPrefix + "-"
	indexTypes := []string{"jaeger-span", "jaeger-service", "jaeger-dependencies", "jaeger-sampling"}
	for _, indexType := range indexTypes {
		initialIndex := prefix + indexType + "-000001"
		writeAlias := prefix + indexType + "-write"
		readAlias := prefix + indexType + "-read"
		createIndexWithAliases(t, client, initialIndex, writeAlias, readAlias)
	}
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, client, prefix)
	})
}

// setupAutoRolloverIndices creates the ILM/ISM policy and initial indices/aliases
// for the auto_rollover strategy.
func setupAutoRolloverIndices(t *testing.T, indexPrefix, policyName string) {
	client := newESClient(t)
	prefix := indexPrefix + "-"

	createTestILMPolicy(t, client, policyName)

	indexTypes := []string{"jaeger-span", "jaeger-service", "jaeger-dependencies", "jaeger-sampling"}
	for _, indexType := range indexTypes {
		initialIndex := prefix + indexType + "-000001"
		writeAlias := prefix + indexType + "-write"
		readAlias := prefix + indexType + "-read"
		createIndexWithAliases(t, client, initialIndex, writeAlias, readAlias)
	}
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, client, prefix)
		deleteILMPolicy(t, policyName)
	})
}

func newESClient(t *testing.T) *elastic.Client {
	client, err := elastic.NewClient(
		elastic.SetURL(esURL),
		elastic.SetSniff(false),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Stop() })
	return client
}

func createIndexWithAliases(t *testing.T, client *elastic.Client, indexName, writeAlias, readAlias string) {
	_, err := client.CreateIndex(indexName).Do(context.Background())
	require.NoError(t, err, "failed to create index %s", indexName)

	_, err = client.Alias().
		Add(indexName, writeAlias).
		AddWithFilter(indexName, readAlias, nil).
		Do(context.Background())
	require.NoError(t, err, "failed to create aliases for %s", indexName)
}

func deleteIndicesByPrefix(t *testing.T, client *elastic.Client, prefix string) {
	_, err := client.DeleteIndex(prefix + "*").Do(context.Background())
	if err != nil && !elastic.IsNotFound(err) {
		t.Logf("warning: failed to delete indices with prefix %s: %v", prefix, err)
	}
}

func createTestILMPolicy(t *testing.T, client *elastic.Client, policyName string) {
	version := detectVersion(t, client)
	if version == "opensearch" {
		createTestISMPolicy(t, policyName)
	} else {
		_, err := client.XPackIlmPutLifecycle().
			Policy(policyName).
			BodyString(`{"policy": {"phases": {"hot": {"min_age": "0ms", "actions": {"rollover": {"max_age": "1d"}}}}}}`).
			Do(context.Background())
		require.NoError(t, err, "failed to create ILM policy %s", policyName)
	}
}

func createTestISMPolicy(t *testing.T, policyName string) {
	body := `{
		"policy": {
			"description": "Jaeger e2e test policy",
			"default_state": "hot",
			"states": [{
				"name": "hot",
				"actions": [{"rollover": {"min_index_age": "1d"}}],
				"transitions": []
			}]
		}
	}`
	url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", esURL, policyName)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK,
		"failed to create ISM policy %s (status %d): %s", policyName, resp.StatusCode, string(respBody))
}

func deleteILMPolicy(t *testing.T, policyName string) {
	version := detectVersion(t, newESClient(t))
	if version == "opensearch" {
		url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", esURL, policyName)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, http.NoBody)
		if err != nil {
			t.Logf("warning: failed to build ISM delete request: %v", err)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("warning: failed to delete ISM policy: %v", err)
			return
		}
		resp.Body.Close()
	} else {
		client := newESClient(t)
		_, err := client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			t.Logf("warning: failed to delete ILM policy: %v", err)
		}
	}
}

func detectVersion(t *testing.T, client *elastic.Client) string {
	ping, _, err := client.Ping(esURL).Do(context.Background())
	require.NoError(t, err)
	if strings.Contains(strings.ToLower(ping.TagLine), "opensearch") ||
		strings.Contains(strings.ToLower(ping.Version.BuildFlavor), "opensearch") {
		return "opensearch"
	}
	return "elasticsearch"
}

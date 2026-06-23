// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

const (
	esHostPort = "localhost:9200"
	esBaseURL  = "http://" + esHostPort
)

// runRotationSmokeTest is a helper that reduces boilerplate for rotation strategy
// e2e tests. It starts Jaeger with the given config and runs the smoke test battery.
func runRotationSmokeTest(t *testing.T, configFile string, storage string) {
	s := &E2EStorageIntegration{
		ConfigFile: configFile,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.ElasticsearchSmokeTest(),
		},
	}
	s.e2eInitialize(t, storage)
	s.RunSpanStoreTests(t)
}

// setupManualRolloverIndices uses the jaeger-es-rollover tool to create the
// initial indices and aliases for the manual_rollover strategy.
func setupManualRolloverIndices(t *testing.T, indexPrefix string) {
	runEsRollover(t, "init", "--index-prefix="+indexPrefix)
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, indexPrefix+"-")
	})
}

// setupAutoRolloverIndices creates an ILM/ISM policy and then uses the
// jaeger-es-rollover tool to create initial indices and aliases for the
// auto_rollover strategy.
func setupAutoRolloverIndices(t *testing.T, indexPrefix, policyName string) {
	createILMPolicy(t, policyName)
	runEsRollover(t, "init",
		"--index-prefix="+indexPrefix,
		"--es.use-ilm=true",
		"--es.ilm-policy-name="+policyName,
	)
	t.Cleanup(func() {
		deleteIndicesByPrefix(t, indexPrefix+"-")
		deleteILMPolicy(t, policyName)
	})
}

func runEsRollover(t *testing.T, action string, flags ...string) {
	args := append([]string{"run", "./cmd/es-rollover", action}, flags...)
	args = append(args, esBaseURL)
	cmd := exec.Command("go", args...)
	cmd.Dir = "../../../.."
	out, err := cmd.CombinedOutput()
	t.Logf("jaeger-es-rollover %s output:\n%s", action, string(out))
	require.NoError(t, err, "jaeger-es-rollover %s failed", action)
}

func createILMPolicy(t *testing.T, policyName string) {
	client := newESClient(t)
	if getBackendVersion(t, client).IsOpenSearch() {
		createISMPolicy(t, policyName)
	} else {
		_, err := client.XPackIlmPutLifecycle().
			Policy(policyName).
			BodyString(`{"policy": {"phases": {"hot": {"min_age": "0ms", "actions": {"rollover": {"max_age": "1d"}}}}}}`).
			Do(context.Background())
		require.NoError(t, err, "failed to create ILM policy %s", policyName)
	}
}

func createISMPolicy(t *testing.T, policyName string) {
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
	url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", esBaseURL, policyName)
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
	client := newESClient(t)
	if getBackendVersion(t, client).IsOpenSearch() {
		url := fmt.Sprintf("%s/_plugins/_ism/policies/%s", esBaseURL, policyName)
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
		_, err := client.XPackIlmDeleteLifecycle().Policy(policyName).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			t.Logf("warning: failed to delete ILM policy: %v", err)
		}
	}
}

func deleteIndicesByPrefix(t *testing.T, prefix string) {
	client := newESClient(t)
	_, err := client.DeleteIndex(prefix + "*").Do(context.Background())
	if err != nil && !elastic.IsNotFound(err) {
		t.Logf("warning: failed to delete indices with prefix %s: %v", prefix, err)
	}
}

func newESClient(t *testing.T) *elastic.Client {
	client, err := elastic.NewClient(
		elastic.SetURL(esBaseURL),
		elastic.SetSniff(false),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Stop() })
	return client
}

func getBackendVersion(t *testing.T, client *elastic.Client) es.BackendVersion {
	ping, _, err := client.Ping(esBaseURL).Do(context.Background())
	require.NoError(t, err)
	parts := strings.SplitN(ping.Version.Number, ".", 2)
	major, err := strconv.Atoi(parts[0])
	require.NoError(t, err)
	return es.DetectBackendVersion(ping.TagLine, major)
}

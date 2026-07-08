// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// esTestClient is a thin test convenience over the owned esclient.Client: it
// issues the handful of admin/inspection calls the integration tests need to set
// up and verify cluster state (create/delete indices, ILM/ISM policies, index
// templates, settings). Every request goes through esclient.Client.Perform — the
// same transport pool + auth/TLS stack the production code uses — so the tests
// talk to the cluster through the one consolidated client, not a second HTTP
// stack. Operations that have no production counterpart (reading flat settings,
// checking template existence, creating a policy) ride Perform directly rather
// than growing the production admin API for test-only needs.
type esTestClient struct {
	t      *testing.T
	client esclient.Client
}

func newESTestClient(t *testing.T) *esTestClient {
	client, err := esclient.NewClient(
		context.Background(),
		&escfg.Configuration{Servers: []string{queryURL}},
		zap.NewNop(), nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return &esTestClient{t: t, client: client}
}

// do issues a request through the shared client's transport and returns the
// response status code and body. The path is relative (e.g. "/_aliases"); the
// pool fills in the node scheme and host. Interpreting the status code is left to
// the caller so 404s can be tolerated where appropriate.
func (c *esTestClient) do(method, path, body string) (int, []byte) {
	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, path, reader)
	require.NoError(c.t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Perform(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp.StatusCode, data
}

// ping returns the raw version number and tagline from the cluster root document.
func (c *esTestClient) ping() (versionNumber, tagLine string) {
	status, data := c.do(http.MethodGet, "/", "")
	require.Equal(c.t, http.StatusOK, status, "ping failed: %s", string(data))
	var info struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
		TagLine string `json:"tagline"`
	}
	require.NoError(c.t, json.Unmarshal(data, &info))
	return info.Version.Number, info.TagLine
}

// majorVersion mirrors the historical getVersion logic: it returns the major
// Elasticsearch version, mapping OpenSearch 1.x/2.x/3.x onto 7 (OpenSearch is
// based on Elasticsearch 7.x for the purposes of the template-API branch).
func (c *esTestClient) majorVersion() uint {
	number, tagLine := c.ping()
	// Parse the whole major component (split on '.'), like ResolveBackendVersion,
	// so multi-digit majors are read correctly and an empty version fails cleanly
	// rather than panicking on number[0].
	major, err := strconv.Atoi(strings.SplitN(number, ".", 2)[0])
	require.NoError(c.t, err)
	// OpenSearch 1.x/2.x/3.x map onto Elasticsearch 7 for template-API selection.
	if strings.Contains(tagLine, "OpenSearch") && major >= 1 && major <= 3 {
		major = 7
	}
	return uint(major)
}

// backendVersion resolves the flavor-aware backend version (Elasticsearch vs
// OpenSearch) the way the production version detection does.
func (c *esTestClient) backendVersion() es.BackendVersion {
	number, tagLine := c.ping()
	major, err := strconv.Atoi(strings.SplitN(number, ".", 2)[0])
	require.NoError(c.t, err)
	return es.DetectBackendVersion(tagLine, major)
}

// deleteIndices deletes every index matching the pattern. A 404 is tolerated so
// callers can clean up unconditionally.
func (c *esTestClient) deleteIndices(pattern string) {
	status, data := c.do(http.MethodDelete, "/"+pattern, "")
	require.True(c.t, status == http.StatusOK || status == http.StatusNotFound,
		"delete indices %q failed (status %d): %s", pattern, status, string(data))
}

// createIndex creates a single empty index.
func (c *esTestClient) createIndex(name string) {
	status, data := c.do(http.MethodPut, "/"+name, "")
	require.Equal(c.t, http.StatusOK, status, "create index %q failed: %s", name, string(data))
}

// indexNames returns the names of all visible indices by reading GET /_aliases
// and returning its keys.
func (c *esTestClient) indexNames() []string {
	status, data := c.do(http.MethodGet, "/_aliases", "")
	require.Equal(c.t, http.StatusOK, status, "get aliases failed: %s", string(data))
	var aliases map[string]any
	require.NoError(c.t, json.Unmarshal(data, &aliases))
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	return names
}

// flatSettings returns the flattened index settings for each named index, keyed
// by index name (the GET /_settings?flat_settings=true form).
func (c *esTestClient) flatSettings(indices []string) map[string]map[string]any {
	status, data := c.do(http.MethodGet, "/"+strings.Join(indices, ",")+"/_settings?flat_settings=true", "")
	require.Equal(c.t, http.StatusOK, status, "get settings failed: %s", string(data))
	var raw map[string]struct {
		Settings map[string]any `json:"settings"`
	}
	require.NoError(c.t, json.Unmarshal(data, &raw))
	out := make(map[string]map[string]any, len(raw))
	for name, entry := range raw {
		out[name] = entry.Settings
	}
	return out
}

// putILMPolicy installs an Elasticsearch ILM lifecycle policy.
func (c *esTestClient) putILMPolicy(policyName, body string) {
	status, data := c.do(http.MethodPut, "/_ilm/policy/"+policyName, body)
	require.Equal(c.t, http.StatusOK, status, "put ILM policy %q failed: %s", policyName, string(data))
}

// deleteILMPolicy removes an Elasticsearch ILM lifecycle policy, tolerating a
// missing policy (404).
func (c *esTestClient) deleteILMPolicy(policyName string) {
	status, data := c.do(http.MethodDelete, "/_ilm/policy/"+policyName, "")
	require.True(c.t, status == http.StatusOK || status == http.StatusNotFound,
		"delete ILM policy %q failed (status %d): %s", policyName, status, string(data))
}

// putISMPolicy installs an OpenSearch ISM policy, tolerating an already-existing
// policy (409).
func (c *esTestClient) putISMPolicy(policyName, body string) {
	status, data := c.do(http.MethodPut, "/_plugins/_ism/policies/"+policyName, body)
	require.True(c.t, status == http.StatusCreated || status == http.StatusOK || status == http.StatusConflict,
		"put ISM policy %q failed (status %d): %s", policyName, status, string(data))
}

// deleteISMPolicy removes an OpenSearch ISM policy, tolerating a missing policy
// (404).
func (c *esTestClient) deleteISMPolicy(policyName string) {
	status, data := c.do(http.MethodDelete, "/_plugins/_ism/policies/"+policyName, "")
	require.True(c.t, status == http.StatusOK || status == http.StatusNotFound,
		"delete ISM policy %q failed (status %d): %s", policyName, status, string(data))
}

// templateExists reports whether the index template for name exists. It uses the
// composable (_index_template) API on ES8+/OpenSearch and the legacy (_template)
// API on older Elasticsearch, mirroring how esclient.CreateTemplate selects the
// endpoint.
func (c *esTestClient) templateExists(name string) bool {
	endpoint := "/_template/" + name
	if c.majorVersion() > 7 {
		endpoint = "/_index_template/" + name
	}
	status, _ := c.do(http.MethodHead, endpoint, "")
	return status == http.StatusOK
}

// deleteTemplates deletes legacy (_template) index templates matching the pattern.
func (c *esTestClient) deleteTemplates(pattern string) {
	status, data := c.do(http.MethodDelete, "/_template/"+pattern, "")
	require.True(c.t, status == http.StatusOK || status == http.StatusNotFound,
		"delete templates %q failed (status %d): %s", pattern, status, string(data))
}

// cleanTemplates removes the span/service/dependency index templates for prefix.
// On ES8+/OpenSearch it deletes the composable (_index_template) templates by
// name; on older Elasticsearch it drops the legacy (_template) templates by
// wildcard — matching what the storage factory installs on each backend.
func (c *esTestClient) cleanTemplates(prefix string) {
	if c.majorVersion() <= 7 {
		c.deleteTemplates("*")
		return
	}
	sep := prefix
	if prefix != "" {
		sep += "-"
	}
	for _, base := range []string{escfg.SpanIndexName, escfg.ServiceIndexName, escfg.DependencyIndexName} {
		status, data := c.do(http.MethodDelete, "/_index_template/"+sep+base, "")
		require.True(c.t, status == http.StatusOK || status == http.StatusNotFound,
			"delete index template %q failed (status %d): %s", sep+base, status, string(data))
	}
}

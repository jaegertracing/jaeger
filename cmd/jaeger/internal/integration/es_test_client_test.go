// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// esAdmin issues the few lifecycle-policy and index cleanup requests the rotation
// e2e tests need. Lifecycle-policy management is not part of Jaeger's production
// client surface — the operator owns ILM/ISM policies — so there is no esclient
// method for it; these test-only requests ride esclient.Client.Perform, the same
// transport the production code uses, rather than a separate HTTP stack. The
// caller supplies the backend flavor (it knows it statically), so nothing here
// probes the cluster for its version.
type esAdmin struct {
	t      *testing.T
	client esclient.Client
}

func newESAdmin(t *testing.T) *esAdmin {
	client, err := esclient.NewClient(
		context.Background(),
		&escfg.Configuration{Servers: []string{esBaseURL}},
		zap.NewNop(), nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return &esAdmin{t: t, client: client}
}

// send issues a relative-path request through the shared client's transport (the
// pool fills in the node scheme and host) and returns the response status code.
func (a *esAdmin) send(method, path, body string) int {
	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, path, reader)
	require.NoError(a.t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Perform(req)
	require.NoError(a.t, err)
	require.NoError(a.t, resp.Body.Close())
	return resp.StatusCode
}

func (a *esAdmin) putILMPolicy(name, body string) {
	require.Equal(a.t, http.StatusOK, a.send(http.MethodPut, "/_ilm/policy/"+name, body),
		"put ILM policy %q", name)
}

func (a *esAdmin) putISMPolicy(name, body string) {
	status := a.send(http.MethodPut, "/_plugins/_ism/policies/"+name, body)
	require.Contains(a.t, []int{http.StatusOK, http.StatusCreated, http.StatusConflict}, status,
		"put ISM policy %q returned status %d", name, status)
}

// The delete* helpers tolerate a missing target (404) so cleanup is idempotent.
func (a *esAdmin) deleteILMPolicy(name string) { a.cleanup(http.MethodDelete, "/_ilm/policy/"+name) }

func (a *esAdmin) deleteISMPolicy(name string) {
	a.cleanup(http.MethodDelete, "/_plugins/_ism/policies/"+name)
}
func (a *esAdmin) deleteIndices(pattern string) { a.cleanup(http.MethodDelete, "/"+pattern) }

func (a *esAdmin) cleanup(method, path string) {
	if status := a.send(method, path, ""); status != http.StatusOK && status != http.StatusNotFound {
		a.t.Logf("warning: %s %s returned status %d", method, path, status)
	}
}

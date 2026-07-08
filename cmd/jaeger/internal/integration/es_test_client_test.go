// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// esAdmin drives the lifecycle-policy setup and index/policy cleanup the rotation
// e2e tests need, through the owned esclient. It delegates to esclient admin
// methods rather than building requests of its own, so the wire formats and
// version gating live in one place. The lifecycle-policy helpers are esclient
// TestsOnly operations: Jaeger never creates policies in production (es-rollover
// requires an operator-created policy to exist).
//
// The helpers take the current *testing.T as their first argument rather than
// capturing one, so assertions attribute to the running test/subtest. They use
// context.Background() rather than t.Context() because deletePolicy and
// deleteJaegerIndices run from t.Cleanup, and t.Context() is already canceled by
// the time cleanup functions run — a canceled context would fail the teardown
// request.
type esAdmin struct {
	client  esclient.Client
	ilm     *esclient.ILMClient
	indices *esclient.IndicesClient
}

func newESAdmin(t *testing.T) *esAdmin {
	client, err := esclient.NewClient(
		context.Background(),
		&escfg.Configuration{Servers: []string{esBaseURL}},
		zap.NewNop(), nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return &esAdmin{
		client:  client,
		ilm:     &esclient.ILMClient{Client: client, Logger: zap.NewNop()},
		indices: &esclient.IndicesClient{Client: client, IgnoreUnavailableIndex: true},
	}
}

// isOpenSearch reports the backend the esclient resolved at construction, used to
// pick the backend-specific policy body (the ILM/ISM endpoint is chosen inside
// esclient).
func (a *esAdmin) isOpenSearch() bool {
	return a.client.TestsOnlyBackendVersion().IsOpenSearch()
}

func (a *esAdmin) putPolicy(t *testing.T, name, body string) {
	require.NoError(t, a.ilm.TestsOnlyPutPolicy(context.Background(), name, body))
}

// deletePolicy and deleteJaegerIndices are cleanup helpers: they log rather than
// fail so a t.Cleanup can run unconditionally.
func (a *esAdmin) deletePolicy(t *testing.T, name string) {
	if err := a.ilm.TestsOnlyDeletePolicy(context.Background(), name); err != nil {
		t.Logf("warning: delete lifecycle policy %q: %v", name, err)
	}
}

func (a *esAdmin) deleteJaegerIndices(t *testing.T, prefix string) {
	indices, err := a.indices.GetJaegerIndices(context.Background(), prefix)
	if err != nil {
		t.Logf("warning: list indices under %q: %v", prefix, err)
		return
	}
	if err := a.indices.DeleteIndices(context.Background(), indices); err != nil {
		t.Logf("warning: delete indices under %q: %v", prefix, err)
	}
}

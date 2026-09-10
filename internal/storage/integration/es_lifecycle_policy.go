// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// The minimal hot-phase rollover policies the ES/OpenSearch rollover tests seed.
// es-rollover with --es.use-ilm only checks that the named policy exists (it never
// creates one), so the tests must create it; the content beyond a rollover action
// is irrelevant to what the tests assert.
const (
	rolloverILMPolicyBody = `{
	"policy": {
		"phases": {
			"hot": {
				"min_age": "0ms",
				"actions": {"rollover": {"max_age": "1d"}}
			}
		}
	}
}`

	rolloverISMPolicyBody = `{
	"policy": {
		"description": "Jaeger integration test rollover policy",
		"default_state": "hot",
		"states": [{
			"name": "hot",
			"actions": [{"rollover": {"min_index_age": "1d"}}],
			"transitions": []
		}]
	}
}`
)

// PutRolloverLifecyclePolicy installs the rollover lifecycle policy the ES/OS
// rollover tests need. It selects the ILM (Elasticsearch) or ISM (OpenSearch)
// body from the backend the client resolved at construction, so callers pass no
// backend flag. Shared by the storage rollover integration tests and the Jaeger
// binary's rotation e2e tests so the policy body lives in one place.
func PutRolloverLifecyclePolicy(t *testing.T, ilm *esclient.ILMClient, name string) {
	body := rolloverILMPolicyBody
	if ilm.TestsOnlyBackendVersion().IsOpenSearch() {
		body = rolloverISMPolicyBody
	}
	require.NoError(t, ilm.TestsOnlyPutPolicy(context.Background(), name, body))
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func CompareJSONSpans(t *testing.T, expected []byte, actual *dbmodel.Span) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(actual))
	if !assert.Equal(t, string(expected), buf.String()) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

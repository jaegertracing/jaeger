// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestEncodeBulkItem(t *testing.T) {
	tests := []struct {
		name       string
		item       BulkItem
		typed      bool
		wantAction string
		wantSource string
	}{
		{
			name:       "span, typed backend",
			item:       BulkItem{Index: "jaeger-span-000001", Type: "span", Body: map[string]any{"traceID": "abc"}},
			typed:      true,
			wantAction: `{"index":{"_index":"jaeger-span-000001","_type":"span"}}`,
			wantSource: `{"traceID":"abc"}`,
		},
		{
			name:       "span, typeless backend drops _type",
			item:       BulkItem{Index: "jaeger-span-000001", Type: "span", Body: map[string]any{"traceID": "abc"}},
			typed:      false,
			wantAction: `{"index":{"_index":"jaeger-span-000001"}}`,
			wantSource: `{"traceID":"abc"}`,
		},
		{
			name:       "service carries _id",
			item:       BulkItem{Index: "svc-000001", Type: "service", ID: "cb42af354c445afb", Body: map[string]any{"serviceName": "s"}},
			typed:      true,
			wantAction: `{"index":{"_id":"cb42af354c445afb","_index":"svc-000001","_type":"service"}}`,
			wantSource: `{"serviceName":"s"}`,
		},
		{
			name:       "create op-type for data streams",
			item:       BulkItem{Index: "jaeger.spans", Type: "span", OpType: es.WriteOpCreate, Body: map[string]any{"traceID": "abc"}},
			typed:      false,
			wantAction: `{"create":{"_index":"jaeger.spans"}}`,
			wantSource: `{"traceID":"abc"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, encodeBulkItem(&buf, tt.item, tt.typed))

			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			require.Len(t, lines, 2)
			assert.JSONEq(t, tt.wantAction, lines[0])
			assert.JSONEq(t, tt.wantSource, lines[1])
			// The body must be newline-terminated so the next pair starts cleanly.
			assert.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")))
		})
	}
}

func TestEncodeBulkItemUnmarshalableBody(t *testing.T) {
	var buf bytes.Buffer
	err := encodeBulkItem(&buf, BulkItem{Index: "i", Body: make(chan int)}, false)
	require.Error(t, err)
}

// TestEncodeBulkItemActionParses is a guard that the action line is a single JSON
// object keyed by the op-type, as the _bulk protocol requires.
func TestEncodeBulkItemActionParses(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, encodeBulkItem(&buf, BulkItem{Index: "i", OpType: es.WriteOpIndex, Body: struct{}{}}, false))
	action := strings.SplitN(buf.String(), "\n", 2)[0]
	var parsed map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(action), &parsed))
	require.Contains(t, parsed, "index")
	assert.Equal(t, "i", parsed["index"]["_index"])
}

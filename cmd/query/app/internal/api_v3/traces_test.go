// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package api_v3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestTracesData(t *testing.T) {
	td := TracesData(ptrace.NewTraces())

	// Test ToTraces
	assert.Equal(t, ptrace.Traces(td), td.ToTraces())

	// Test Marshal
	_, err := td.Marshal()
	require.NoError(t, err)

	// Test MarshalTo
	assert.Panics(t, func() { td.MarshalTo(nil) })

	// Test MarshalJSONPB
	_, err = td.MarshalJSONPB(nil)
	require.NoError(t, err)

	// Test UnmarshalJSONPB
	err = td.UnmarshalJSONPB(nil, []byte(`{"resourceSpans":[]}`))
	require.NoError(t, err)

	err = td.UnmarshalJSONPB(nil, []byte(`{"resourceSpans":123}`))
	require.Error(t, err)

	// Test Size
	assert.Equal(t, 0, td.Size())

	// Test Unmarshal
	err = td.Unmarshal([]byte{})
	require.NoError(t, err)
	err = td.Unmarshal([]byte{1})
	require.Error(t, err)

	// Test ProtoMessage
	td.ProtoMessage()

	// Test Reset
	td.Reset()
	assert.Equal(t, TracesData(ptrace.NewTraces()), td)

	// Test String
	assert.Equal(t, "*TracesData", td.String())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

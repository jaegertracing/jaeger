// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestTracesData(t *testing.T) {
	td := TracesData(ptrace.NewTraces())

	// Test ToTraces
	assert.Equal(t, ptrace.Traces(td), td.ToTraces())

	// Test Marshal
	_, err := td.Marshal()
	require.NoError(t, err)

	// Test MarshalTo
	_, err = td.MarshalTo(make([]byte, td.Size()))
	require.NoError(t, err)

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

// TestTracesDataMarshalToSizedBufferWritesAtTail pins the gogo sized-buffer convention: a caller
// marshaling a parent message passes a buffer with room reserved for fields it has not written
// yet, ahead of this field's own space, and expects this field's bytes to land at the buffer's
// tail, ending exactly at its end. A field that instead writes from the buffer's start (as this
// one once did) clobbers that reserved room whenever it is nested inside another message on the
// binary wire, rather than marshaled as the top-level message.
func TestTracesDataMarshalToSizedBufferWritesAtTail(t *testing.T) {
	trace := ptrace.NewTraces()
	span := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("a-span")
	td := TracesData(trace)

	size := td.Size()
	require.Positive(t, size)

	const reserved = 7
	buf := make([]byte, reserved+size)
	sentinel := bytes.Repeat([]byte{0xFF}, reserved)
	copy(buf[:reserved], sentinel)

	n, err := td.MarshalToSizedBuffer(buf)
	require.NoError(t, err)
	assert.Equal(t, size, n)
	assert.Equal(t, sentinel, buf[:reserved], "the reserved room ahead of this field must be untouched")

	var decoded TracesData
	require.NoError(t, decoded.Unmarshal(buf[reserved:]))
	assert.Equal(t, 1, decoded.ToTraces().SpanCount())
}

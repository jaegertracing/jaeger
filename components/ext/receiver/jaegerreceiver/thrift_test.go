// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerreceiver_test

import (
	"context"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
)

// TestBatchRejectsSpanCountBeyondInput pins the Thrift version that Jaeger's go.mod resolves for
// the receiver: a list header that declares more spans than the input holds is refused before
// the decoder sizes the span slice from it.
func TestBatchRejectsSpanCountBeyondInput(t *testing.T) {
	const declaredSpans = 1_000_000

	protocols := []struct {
		name string
		new  func(thrift.TTransport) thrift.TProtocol
	}{
		{
			name: "binary",
			new: func(transport thrift.TTransport) thrift.TProtocol {
				return thrift.NewTBinaryProtocolTransport(transport)
			},
		},
		{
			name: "compact",
			new: func(transport thrift.TTransport) thrift.TProtocol {
				return thrift.NewTCompactProtocol(transport)
			},
		},
	}

	for _, protocol := range protocols {
		t.Run(protocol.name, func(t *testing.T) {
			transport := thrift.NewTMemoryBuffer()
			writer := protocol.new(transport)
			require.NoError(t, writer.WriteListBegin(context.Background(), thrift.STRUCT, declaredSpans))
			require.NoError(t, writer.Flush(context.Background()))

			batch := &jaeger.Batch{}
			err := batch.ReadField2(context.Background(), protocol.new(transport))

			var protocolErr thrift.TProtocolException
			require.ErrorAs(t, err, &protocolErr)
			require.Equal(t, thrift.SIZE_LIMIT, protocolErr.TypeId())
			require.Nil(t, batch.Spans)
		})
	}
}

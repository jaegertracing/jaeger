// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package zipkinreceiver_test

import (
	"context"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
)

// TestSpanRejectsListCountBeyondInput pins the Thrift version that Jaeger's go.mod resolves for
// the Zipkin v1 Thrift decoder: a span's annotation lists that declare more elements than the
// input holds are refused before the decoder sizes the slice from them.
func TestSpanRejectsListCountBeyondInput(t *testing.T) {
	const declaredElements = 1_000_000

	fields := []struct {
		name string
		read func(*zipkincore.Span, thrift.TProtocol) error
	}{
		{
			name: "annotations",
			read: func(span *zipkincore.Span, protocol thrift.TProtocol) error {
				return span.ReadField6(context.Background(), protocol)
			},
		},
		{
			name: "binary annotations",
			read: func(span *zipkincore.Span, protocol thrift.TProtocol) error {
				return span.ReadField8(context.Background(), protocol)
			},
		},
	}

	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			transport := thrift.NewTMemoryBuffer()
			writer := thrift.NewTBinaryProtocolTransport(transport)
			require.NoError(t, writer.WriteListBegin(context.Background(), thrift.STRUCT, declaredElements))
			require.NoError(t, writer.Flush(context.Background()))

			span := &zipkincore.Span{}
			err := field.read(span, thrift.NewTBinaryProtocolTransport(transport))

			var protocolErr thrift.TProtocolException
			require.ErrorAs(t, err, &protocolErr)
			require.Equal(t, thrift.SIZE_LIMIT, protocolErr.TypeId())
			require.Nil(t, span.Annotations)
			require.Nil(t, span.BinaryAnnotations)
		})
	}
}

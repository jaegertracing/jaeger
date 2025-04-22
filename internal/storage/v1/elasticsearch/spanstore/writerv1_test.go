// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
)

func TestSpanWriterV1_WriteSpan(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	s := &model.Span{
		Tags:    []model.KeyValue{{Key: "foo", VStr: "bar"}},
		Process: &model.Process{Tags: []model.KeyValue{{Key: "bar", VStr: "baz"}}},
	}
	writerV1 := &SpanWriterV1{spanWriter: coreWriter}
	coreWriter.On("WriteSpan", s.StartTime, FromDomainEmbedProcess(s))
	err := writerV1.WriteSpan(context.Background(), s)
	require.NoError(t, err)
}

func TestSpanWriterV1_Close(t *testing.T) {
	coreWriter := &mocks.CoreSpanWriter{}
	coreWriter.On("Close").Return(nil)
	writerV1 := &SpanWriterV1{spanWriter: coreWriter}
	err := writerV1.Close()
	require.NoError(t, err)
}

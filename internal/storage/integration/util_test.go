// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

func Test_translateFixtureToOTLPTrace(t *testing.T) {
	tmpDir := t.TempDir()
	tmpInFile := filepath.Join(tmpDir, "v1-trace.json")
	v1Trace := model.Trace{Spans: []*model.Span{
		{
			OperationName: "jaeger",
			References:    []model.SpanRef{},
			Process: &model.Process{
				Tags:        model.KeyValues{},
				ServiceName: "jaeger-service",
			},
			Tags:      model.KeyValues{},
			StartTime: time.Now(),
			Logs:      []model.Log{},
		},
	}}
	var buf bytes.Buffer
	marshaler := jsonpb.Marshaler{}
	require.NoError(t, marshaler.Marshal(&buf, &v1Trace))
	require.NoError(t, os.WriteFile(tmpInFile, buf.Bytes(), 0o600))
	tmpOutFile := filepath.Join(tmpDir, "v2-trace.json")
	require.NoError(t, translateFixtureToOTLPTrace(tmpInFile, tmpOutFile))
	outBytes, err := os.ReadFile(tmpOutFile)
	require.NoError(t, err)
	unmarshaller := ptrace.JSONUnmarshaler{}
	actual, err := unmarshaller.UnmarshalTraces(outBytes)
	require.NoError(t, err)
	expected := v1adapter.V1TraceToOtelTrace(&v1Trace)
	CompareTraces(t, expected, actual)
}

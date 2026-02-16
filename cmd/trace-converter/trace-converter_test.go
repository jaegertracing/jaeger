// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

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
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func Test_translateFixtureToOTLPTrace(t *testing.T) {
	tmpInFile := "v1-trace.json"
	tmpInPath := filepath.Join(rootPathVal, tmpInFile)
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
	require.NoError(t, os.WriteFile(tmpInPath, buf.Bytes(), 0o600))
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmpInPath))
	})
	tmpOutFile := "v2-trace.json"
	tmpOutPath := filepath.Join(rootPathVal, tmpOutFile)
	rootCmd := newRootCmd()
	rootCmd.SetArgs([]string{
		"--in-file", tmpInFile,
		"--out-file", tmpOutFile,
	})
	require.NoError(t, rootCmd.Execute())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmpOutPath))
	})
	outBytes, err := os.ReadFile(tmpOutPath)
	require.NoError(t, err)
	unmarshaller := ptrace.JSONUnmarshaler{}
	actual, err := unmarshaller.UnmarshalTraces(outBytes)
	require.NoError(t, err)
	expected := v1adapter.V1TraceToOtelTrace(&v1Trace)
	integration.CompareTraces(t, expected, actual)
}

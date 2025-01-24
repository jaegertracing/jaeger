// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanRefTypeToFromJSON(t *testing.T) {
	// base64(0x42, 16 bytes) == AAAAAAAAAAAAAAAAAAAAQg==
	// base64(0x43, 8 bytes) == AAAAAAAAAEM=
	// Verify: https://cryptii.com/pipes/base64-to-hex
	sr := model.SpanRef{
		TraceID: model.NewTraceID(0, 0x42),
		SpanID:  model.NewSpanID(0x43),
		RefType: model.FollowsFrom,
	}
	out := new(bytes.Buffer)
	err := new(jsonpb.Marshaler).Marshal(out, &sr)
	require.NoError(t, err)
	assert.JSONEq(t, `{"traceId":"AAAAAAAAAAAAAAAAAAAAQg==","spanId":"AAAAAAAAAEM=","refType":"FOLLOWS_FROM"}`, out.String())
	var sr2 model.SpanRef
	require.NoError(t, jsonpb.Unmarshal(out, &sr2))
	assert.Equal(t, sr, sr2)
	var sr3 model.SpanRef
	err = jsonpb.Unmarshal(bytes.NewReader([]byte(`{"refType":"BAD"}`)), &sr3)
	assert.ErrorContains(t, err, "unknown value")
}

func TestMaybeAddParentSpanID(t *testing.T) {
	span := makeSpan(model.String("k", "v"))
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.References = model.MaybeAddParentSpanID(span.TraceID, model.NewSpanID(0), span.References)
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.References = model.MaybeAddParentSpanID(span.TraceID, model.NewSpanID(123), span.References)
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.References = model.MaybeAddParentSpanID(span.TraceID, model.NewSpanID(123), []model.SpanRef{})
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.References = []model.SpanRef{model.NewChildOfRef(model.NewTraceID(42, 0), model.NewSpanID(789))}
	span.References = model.MaybeAddParentSpanID(span.TraceID, model.NewSpanID(123), span.References)
	assert.Equal(t, model.NewSpanID(123), span.References[0].SpanID, "parent added as first reference")
}

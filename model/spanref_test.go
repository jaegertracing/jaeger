// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestSpanRefTypeToFromJSON(t *testing.T) {
	// base64(0x42, 16 bytes) == AAAAAAAAAAAAAAAAAAAAQg==
	// base64(0x43, 8 bytes) == AAAAAAAAAEM=
	// Verify: https://cryptii.com/base64-to-hex
	sr := model.SpanRef{
		TraceID: model.NewTraceID(0, 0x42),
		SpanID:  model.NewSpanID(0x43),
		RefType: model.FollowsFrom,
	}
	out := new(bytes.Buffer)
	err := new(jsonpb.Marshaler).Marshal(out, &sr)
	assert.NoError(t, err)
	assert.Equal(t, `{"traceId":"AAAAAAAAAAAAAAAAAAAAQg==","spanId":"AAAAAAAAAEM=","refType":"FOLLOWS_FROM"}`, out.String())
	var sr2 model.SpanRef
	if assert.NoError(t, jsonpb.Unmarshal(out, &sr2)) {
		assert.Equal(t, sr, sr2)
	}
	var sr3 model.SpanRef
	err = jsonpb.Unmarshal(bytes.NewReader([]byte(`{"refType":"BAD"}`)), &sr3)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "unknown value")
	}
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

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
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestSpanRefTypeToFromString(t *testing.T) {
	badSpanRefType := model.SpanRefType(-1)
	testCases := []struct {
		v model.SpanRefType
		s string
	}{
		{model.ChildOf, "CHILD_OF"},
		{model.FollowsFrom, "FOLLOWS_FROM"},
		{badSpanRefType, "-1"},
	}
	for _, testCase := range testCases {
		assert.Equal(t, testCase.s, testCase.v.String(), testCase.s)
		v2, err := model.SpanRefTypeFromString(testCase.s)
		if testCase.v == badSpanRefType {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err, testCase.s)
			assert.Equal(t, testCase.v, v2, testCase.s)
		}
	}
}

func TestSpanRefTypeToFromJSON(t *testing.T) {
	sr := model.SpanRef{
		TraceID: model.TraceID{Low: 0x42},
		SpanID:  model.SpanID(0x43),
		RefType: model.FollowsFrom,
	}
	out := new(bytes.Buffer)
	err := new(jsonpb.Marshaler).Marshal(out, &sr)
	assert.NoError(t, err)
	assert.Equal(t, `{"traceId":"42","spanId":"43","refType":"FOLLOWS_FROM"}`, out.String())
	var sr2 model.SpanRef
	if assert.NoError(t, jsonpb.Unmarshal(out, &sr2)) {
		assert.Equal(t, sr, sr2)
	}
	var sr3 model.SpanRef
	err = jsonpb.Unmarshal(bytes.NewReader([]byte(`{"refType":"BAD"}`)), &sr3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown value")
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

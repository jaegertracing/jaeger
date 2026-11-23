// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/uimodel"
)

const NumberOfFixtures = 1

func TestMarshalJSON(t *testing.T) {
	span1 := &model.Span{
		TraceID:       model.TraceID{Low: 1},
		SpanID:        model.SpanID(2),
		OperationName: "span",
		StartTime:     time.Now(),
		Duration:      time.Microsecond,
	}
	trace1 := &model.Trace{
		Spans: []*model.Span{
			span1,
		},
		ProcessMap: []model.Trace_ProcessMapping{
			{
				ProcessID: "p1",
				Process: model.Process{
					ServiceName: "abc",
				},
			},
		},
	}
	m := &jsonpb.Marshaler{}
	out := &bytes.Buffer{}

	require.NoError(t, m.Marshal(out, trace1))

	var trace2 model.Trace
	bb := bytes.NewReader(out.Bytes())
	require.NoError(t, jsonpb.Unmarshal(bb, &trace2))
	trace1.NormalizeTimestamps()
	trace2.NormalizeTimestamps()
	assert.Equal(t, &trace2, trace1)
}

func TestFromDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		domainStr, jsonStr := loadFixturesUI(t, i)

		var trace model.Trace
		require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(domainStr), &trace))
		uiTrace := FromDomain(&trace)

		testJSONEncoding(t, i, jsonStr, uiTrace, false)
	}
}

func TestFromDomainEmbedProcess(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		domainStr, jsonStr := loadFixturesES(t, i)

		var span model.Span
		require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(domainStr), &span))
		embeddedSpan := FromDomainEmbedProcess(&span)

		var expectedSpan uimodel.Span
		require.NoError(t, json.Unmarshal(jsonStr, &expectedSpan))

		testJSONEncoding(t, i, jsonStr, embeddedSpan, true)

		CompareJSONSpans(t, &expectedSpan, embeddedSpan)
	}
}

func loadFixturesUI(t *testing.T, i int) (inStr []byte, outStr []byte) {
	return loadFixtures(t, i, false)
}

func loadFixturesES(t *testing.T, i int) (inStr []byte, outStr []byte) {
	return loadFixtures(t, i, true)
}

// Loads and returns domain model and JSON model fixtures with given number i.
func loadFixtures(t *testing.T, i int, processEmbedded bool) (inStr []byte, outStr []byte) {
	var in string
	var err error
	if processEmbedded {
		in = fmt.Sprintf("fixtures/domain_es_%02d.json", i)
	} else {
		in = fmt.Sprintf("fixtures/domain_%02d.json", i)
	}
	inStr, err = os.ReadFile(in)
	require.NoError(t, err)
	var out string
	if processEmbedded {
		out = fmt.Sprintf("fixtures/es_%02d.json", i)
	} else {
		out = fmt.Sprintf("fixtures/ui_%02d.json", i)
	}
	outStr, err = os.ReadFile(out)
	require.NoError(t, err)
	return inStr, outStr
}

func testJSONEncoding(t *testing.T, i int, expectedStr []byte, object any, processEmbedded bool) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	var outFile string
	if processEmbedded {
		outFile = fmt.Sprintf("fixtures/es_%02d", i)
	} else {
		outFile = fmt.Sprintf("fixtures/ui_%02d", i)
	}
	require.NoError(t, enc.Encode(object))

	if !assert.Equal(t, string(expectedStr), buf.String()) {
		err := os.WriteFile(outFile+"-actual.json", buf.Bytes(), 0o644)
		require.NoError(t, err)
	}
}

func TestDependenciesFromDomain(t *testing.T) {
	someParent := "someParent"
	someChild := "someChild"
	someCallCount := uint64(123)
	anotherParent := "anotherParent"
	anotherChild := "anotherChild"
	anotherCallCount := uint64(456)
	expected := []uimodel.DependencyLink{
		{
			Parent:    someParent,
			Child:     someChild,
			CallCount: someCallCount,
		},
		{
			Parent:    anotherParent,
			Child:     anotherChild,
			CallCount: anotherCallCount,
		},
	}
	input := []model.DependencyLink{
		{
			Parent:    someParent,
			Child:     someChild,
			CallCount: someCallCount,
		},
		{
			Parent:    anotherParent,
			Child:     anotherChild,
			CallCount: anotherCallCount,
		},
	}
	actual := DependenciesFromDomain(input)
	assert.Equal(t, expected, actual)
}

func TestConvertKeyValues_DefaultValueType(t *testing.T) {
	// Create a custom ValueType that's not handled by the switch
	customType := model.ValueType(999)

	kv := model.KeyValue{
		Key:   "custom-key",
		VType: customType,
		VStr:  "custom-value",
	}

	fd := fromDomain{}
	result := fd.convertKeyValues(model.KeyValues{kv})

	require.Len(t, result, 1)
	assert.Equal(t, "custom-key", result[0].Key)

	assert.Equal(t, "unknown type 999", result[0].Value)
	assert.Equal(t, uimodel.ValueType("999"), result[0].Type)
}

func TestFromDomainNonFiniteFloat64(t *testing.T) {
	kvs := func() model.KeyValues {
		return model.KeyValues{
			model.Float64("nan", math.NaN()),
			model.Float64("pos-inf", math.Inf(1)),
			model.Float64("neg-inf", math.Inf(-1)),
			model.Float64("finite", 1.5),
		}
	}
	trace := &model.Trace{
		Spans: []*model.Span{{
			TraceID:   model.TraceID{Low: 1},
			SpanID:    model.SpanID(2),
			StartTime: time.Now(),
			Tags:      kvs(),
			Logs:      []model.Log{{Timestamp: time.Now(), Fields: kvs()}},
			Process:   model.NewProcess("svc", kvs()),
		}},
	}

	out, err := json.Marshal(FromDomain(trace))
	require.NoError(t, err)

	// Keep values as raw JSON so a string can be told apart from a number.
	type rawKV struct {
		Key   string          `json:"key"`
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	var decoded struct {
		Spans []struct {
			Tags []rawKV `json:"tags"`
			Logs []struct {
				Fields []rawKV `json:"fields"`
			} `json:"logs"`
		} `json:"spans"`
		Processes map[string]struct {
			Tags []rawKV `json:"tags"`
		} `json:"processes"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	require.Len(t, decoded.Spans, 1)
	require.Len(t, decoded.Spans[0].Logs, 1)
	require.Len(t, decoded.Processes, 1)

	expected := map[string]string{
		"nan":     `"NaN"`,
		"pos-inf": `"Infinity"`,
		"neg-inf": `"-Infinity"`,
		"finite":  `1.5`,
	}
	check := func(t *testing.T, got []rawKV) {
		require.Len(t, got, len(expected))
		for _, kv := range got {
			assert.Equal(t, "float64", kv.Type, kv.Key)
			assert.JSONEq(t, expected[kv.Key], string(kv.Value), kv.Key)
		}
	}
	t.Run("span tags", func(t *testing.T) { check(t, decoded.Spans[0].Tags) })
	t.Run("log fields", func(t *testing.T) { check(t, decoded.Spans[0].Logs[0].Fields) })
	for _, p := range decoded.Processes {
		t.Run("process tags", func(t *testing.T) { check(t, p.Tags) })
	}
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func TestToDomain(t *testing.T) {
	testToDomain(t, false)
	testToDomain(t, true)
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), int64(math.Float64bits(72.5)))
}

func testToDomain(t *testing.T, testParentSpanID bool) {
	for i := 1; i <= NumberOfFixtures; i++ {
		span, err := loadESSpanFixture(i)
		require.NoError(t, err)
		if testParentSpanID {
			span.ParentSpanID = "3"
		}

		actualSpan, err := NewToDomain().SpanToDomain(&span)
		require.NoError(t, err)

		out := fmt.Sprintf("fixtures/domain_%02d.json", i)
		outStr, err := os.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(outStr), &expectedSpan))

		CompareModelSpans(t, &expectedSpan, actualSpan)
	}
}

func loadESSpanFixture(i int) (dbmodel.Span, error) {
	in := fmt.Sprintf("fixtures/es_%02d.json", i)
	inStr, err := os.ReadFile(in)
	if err != nil {
		return dbmodel.Span{}, err
	}
	var span dbmodel.Span
	err = json.Unmarshal(inStr, &span)
	return span, err
}

func failingSpanTransform(t *testing.T, embeddedSpan *dbmodel.Span, errMsg string) {
	domainSpan, err := NewToDomain().SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	require.EqualError(t, err, errMsg)
}

func failingSpanTransformAnyMsg(t *testing.T, embeddedSpan *dbmodel.Span) {
	domainSpan, err := NewToDomain().SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	require.Error(t, err)
}

func TestFailureBadTypeTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []dbmodel.KeyValue{
		{
			Key:   "meh",
			Type:  "badType",
			Value: "",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadBoolTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []dbmodel.KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "bool",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadIntTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []dbmodel.KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "int64",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadFloatTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []dbmodel.KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "float64",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadBinaryTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []dbmodel.KeyValue{
		{
			Key:   "zzzz",
			Value: "zzzz",
			Type:  "binary",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadLogs(t *testing.T) {
	badLogsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badLogsESSpan.Logs = []dbmodel.Log{
		{
			Timestamp: 0,
			Fields: []dbmodel.KeyValue{
				{
					Key:   "sneh",
					Value: "",
					Type:  "badType",
				},
			},
		},
	}
	failingSpanTransform(t, &badLogsESSpan, "not a valid ValueType string badType")
}

func TestRevertKeyValueOfType(t *testing.T) {
	tests := []struct {
		name  string
		kv    *dbmodel.KeyValue
		err   string
		outKv model.KeyValue
	}{
		{
			name: "not a valid ValueType string",
			kv: &dbmodel.KeyValue{
				Key:   "sneh",
				Type:  "badType",
				Value: "someString",
			},
			err: "not a valid ValueType string",
		},
		{
			name: "invalid nil Value",
			kv:   &dbmodel.KeyValue{},
			err:  "invalid nil Value",
		},
		{
			name: "right int value",
			kv: &dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: int64(123),
			},
			outKv: model.KeyValue{
				Key:    "int-val",
				VInt64: 123,
				VType:  2,
			},
		},
		{
			name: "right int float value",
			kv: &dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: float64(123),
			},
			outKv: model.KeyValue{
				Key:    "int-val",
				VInt64: 123,
				VType:  2,
			},
		},
		{
			name: "right int json number",
			kv: &dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: json.Number("123"),
			},
			outKv: model.KeyValue{
				Key:    "int-val",
				VInt64: 123,
				VType:  2,
			},
		},
		{
			name: "right float value",
			kv: &dbmodel.KeyValue{
				Key:   "float-val",
				Type:  dbmodel.Float64Type,
				Value: 123.4,
			},
			outKv: model.KeyValue{
				Key:      "float-val",
				VFloat64: 123.4,
				VType:    3,
			},
		},
		{
			name: "right float json number",
			kv: &dbmodel.KeyValue{
				Key:   "float-val",
				Type:  dbmodel.Float64Type,
				Value: json.Number("123.4"),
			},
			outKv: model.KeyValue{
				Key:      "float-val",
				VFloat64: 123.4,
				VType:    3,
			},
		},
		{
			name: "wrong int64 value",
			kv: &dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: true,
			},
			err: "invalid int64 type in true",
		},
		{
			name: "wrong float64 value",
			kv: &dbmodel.KeyValue{
				Key:   "float-val",
				Type:  dbmodel.Float64Type,
				Value: true,
			},
			err: "invalid float64 type in true",
		},
		{
			name: "wrong bool value",
			kv: &dbmodel.KeyValue{
				Key:   "bool-val",
				Type:  dbmodel.BoolType,
				Value: 1.23,
			},
			err: "invalid bool type in 1.23",
		},
		{
			name: "wrong string value",
			kv: &dbmodel.KeyValue{
				Key:   "string-val",
				Type:  dbmodel.StringType,
				Value: 123,
			},
			err: "invalid string type in 123",
		},
	}
	td := ToDomain{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag := test.kv
			out, err := td.convertKeyValue(tag)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
			}
			assert.Equal(t, test.outKv, out)
		})
	}
}

func TestFailureBadRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []dbmodel.Reference{
		{
			RefType: "makeOurOwnCasino",
			TraceID: "1",
		},
	}
	failingSpanTransform(t, &badRefsESSpan, "not a valid SpanRefType string makeOurOwnCasino")
}

func TestFailureBadTraceIDRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []dbmodel.Reference{
		{
			RefType: "CHILD_OF",
			TraceID: "ZZBADZZ",
			SpanID:  "1",
		},
	}
	failingSpanTransformAnyMsg(t, &badRefsESSpan)
}

func TestFailureBadSpanIDRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []dbmodel.Reference{
		{
			RefType: "CHILD_OF",
			TraceID: "1",
			SpanID:  "ZZBADZZ",
		},
	}
	failingSpanTransformAnyMsg(t, &badRefsESSpan)
}

func TestFailureBadProcess(t *testing.T) {
	badProcessESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTags := []dbmodel.KeyValue{
		{
			Key:   "meh",
			Value: "",
			Type:  "badType",
		},
	}
	badProcessESSpan.Process = dbmodel.Process{
		ServiceName: "hello",
		Tags:        badTags,
	}
	failingSpanTransform(t, &badProcessESSpan, "not a valid ValueType string badType")
}

func TestFailureBadTraceID(t *testing.T) {
	badTraceIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badTraceIDESSpan.TraceID = "zz"
	failingSpanTransformAnyMsg(t, &badTraceIDESSpan)
}

func TestFailureBadSpanID(t *testing.T) {
	badSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badSpanIDESSpan.SpanID = "zz"
	failingSpanTransformAnyMsg(t, &badSpanIDESSpan)
}

func TestFailureBadParentSpanID(t *testing.T) {
	badParentSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.ParentSpanID = "zz"
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}

func CompareModelSpans(t *testing.T, expected *model.Span, actual *model.Span) {
	model.SortSpan(expected)
	model.SortSpan(actual)

	if !assert.Equal(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

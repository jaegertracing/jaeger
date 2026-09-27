// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// The request types and the reader's query types are converted field by field, so a field added
// to either side compiles without being carried across. These tests walk the fields by reflection
// so that such a field fails a test instead: a request field must reach the reader's query under
// the same name unless it is listed as request-only, and a reader field must have a request field
// that feeds it.

// requestOnlyTraceFields are the trace request fields that have no counterpart in the reader's
// query. Each is asserted to be absent from tracestore.TraceQueryParams, so the list cannot hide a
// field that gained one.
var requestOnlyTraceFields = map[string]bool{
	"RawTraces": true, // presentation, applied after the reader answers
}

// distinctValue returns a value of the field's type that is not the zero value and stays inside
// the range toReaderQuery accepts alongside a bare time window. A field of a type this function
// does not know fails the test, which is the point: the new field needs a generator and a mapping.
func distinctValue(t *testing.T, field reflect.StructField) reflect.Value {
	t.Helper()
	switch field.Type {
	case reflect.TypeOf(""):
		return reflect.ValueOf(field.Name)
	case reflect.TypeOf(0):
		return reflect.ValueOf(7)
	case reflect.TypeOf(true):
		return reflect.ValueOf(true)
	case reflect.TypeOf(time.Duration(0)):
		return reflect.ValueOf(7 * time.Millisecond)
	case reflect.TypeOf(time.Time{}):
		// Inside the base window, so it is valid as either bound.
		return reflect.ValueOf(testWindowStart.Add(30 * time.Second))
	case reflect.TypeOf(pcommon.Map{}):
		m := pcommon.NewMap()
		m.PutStr("k", "v")
		return reflect.ValueOf(m)
	case reflect.TypeOf((*expression.Call)(nil)):
		return reflect.ValueOf(serviceIs("cart"))
	case reflect.TypeOf((*Pagination)(nil)):
		return reflect.ValueOf(&Pagination{PageSize: 7, PageToken: "token"})
	case reflect.TypeOf(Pagination{}):
		return reflect.ValueOf(Pagination{PageSize: 7, PageToken: "token"})
	}
	t.Fatalf("no distinct value for field %s of type %s; add one and map the field", field.Name, field.Type)
	return reflect.Value{}
}

// assertCarried checks that the request field reached the reader's query. Pagination is the one
// field whose type differs between the two, so it is compared member by member.
func assertCarried(t *testing.T, field reflect.StructField, want, got reflect.Value) {
	t.Helper()
	if field.Name != "Pagination" {
		assert.Equal(t, want.Interface(), got.Interface(), field.Name)
		return
	}
	want, got = reflect.Indirect(want), reflect.Indirect(got)
	require.True(t, got.IsValid(), "Pagination reached the reader as nil")
	assert.Equal(t, want.FieldByName("PageSize").Interface(), got.FieldByName("PageSize").Interface())
	assert.Equal(t, want.FieldByName("PageToken").Interface(), got.FieldByName("PageToken").String())
}

func TestTraceQueryParams_EveryFieldReachesTheReader(t *testing.T) {
	enablePagination(t)
	requestType := reflect.TypeOf(TraceQueryParams{})
	readerType := reflect.TypeOf(tracestore.TraceQueryParams{})

	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			if requestOnlyTraceFields[field.Name] {
				_, ok := readerType.FieldByName(field.Name)
				assert.False(t, ok, "%s is listed as request-only but the reader's query has it", field.Name)
				return
			}
			request := TraceQueryParams{StartTimeMin: testWindowStart, StartTimeMax: testWindowEnd}
			reflect.ValueOf(&request).Elem().Field(i).Set(distinctValue(t, field))

			query, err := request.toReaderQuery()
			require.NoError(t, err)
			got := reflect.ValueOf(query).FieldByName(field.Name)
			require.True(t, got.IsValid(), "the reader's query has no field %s", field.Name)
			assertCarried(t, field, reflect.ValueOf(request).Field(i), got)
		})
	}
	for i := 0; i < readerType.NumField(); i++ {
		name := readerType.Field(i).Name
		_, ok := requestType.FieldByName(name)
		assert.True(t, ok, "the reader's query field %s has no request field feeding it", name)
	}
}

func TestSpanQueryParams_EveryFieldReachesTheReader(t *testing.T) {
	enablePagination(t)
	requestType := reflect.TypeOf(SpanQueryParams{})
	readerType := reflect.TypeOf(tracestore.SpanQueryParams{})

	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			request := SpanQueryParams{StartTimeMin: testWindowStart, StartTimeMax: testWindowEnd}
			reflect.ValueOf(&request).Elem().Field(i).Set(distinctValue(t, field))

			query, err := request.toReaderQuery()
			require.NoError(t, err)
			got := reflect.ValueOf(query).FieldByName(field.Name)
			require.True(t, got.IsValid(), "the reader's query has no field %s", field.Name)
			assertCarried(t, field, reflect.ValueOf(request).Field(i), got)
		})
	}
	for i := 0; i < readerType.NumField(); i++ {
		name := readerType.Field(i).Name
		_, ok := requestType.FieldByName(name)
		assert.True(t, ok, "the reader's query field %s has no request field feeding it", name)
	}
}

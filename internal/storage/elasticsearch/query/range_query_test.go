// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"encoding/json"
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func assertRangeQuery(t *testing.T, q *RangeQuery, expected string) {
	t.Helper()
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	if got != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestRangeQuery(t *testing.T) {
	q := NewRangeQuery("postDate").
		Gte("2010-03-01").
		Lte("2010-04-01").
		Boost(3).
		Relation("within").
		QueryName("my_query")

	expected := `{"range":{"_name":"my_query","postDate":{"boost":3,"gte":"2010-03-01","lte":"2010-04-01","relation":"within"}}}`
	assertRangeQuery(t, q, expected)
}

func TestRangeQueryWithTimeZone(t *testing.T) {
	q := NewRangeQuery("born").
		Gte("2012-01-01").
		Lte("now").
		TimeZone("+1:00")

	expected := `{"range":{"born":{"gte":"2012-01-01","lte":"now","time_zone":"+1:00"}}}`
	assertRangeQuery(t, q, expected)
}

func TestRangeQueryWithFormat(t *testing.T) {
	q := NewRangeQuery("born").
		Gt("2012/01/01").
		Lt("now").
		Format("yyyy/MM/dd")

	expected := `{"range":{"born":{"format":"yyyy/MM/dd","gt":"2012/01/01","lt":"now"}}}`
	assertRangeQuery(t, q, expected)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

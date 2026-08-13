// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// This file measures the typed attribute sub-field RFC 0015 proposes against a
// live backend, on both Elasticsearch and OpenSearch. The sub-field exists so a
// query can *order* on an attribute, which a keyword mapping cannot do, and the
// questions the RFC leaves open in §7 are all about what a value actually lands
// in. Those answers come from the engine, so they belong in an integration test
// rather than a rendered-template assertion.
//
// Every probe writes the same attribute key at a different type, so the run also
// exercises the RFC's claim that a key emitted at several types by different
// services indexes correctly for each of them, with no type assigned to the key.

const (
	probeKey   = "probe"
	probeIndex = indexPrefix + "-jaeger-span-*"

	// doublePrecisionLimit is the largest integer below which a double represents
	// every integer exactly. The probe writes one more than this, which the double
	// sub-field cannot hold and rounds back down to it.
	doublePrecisionLimit = int64(1) << 53 // 9007199254740992
)

// longNumericString exceeds the keyword mapping's ignore_above bound while still
// parsing as a number, which is the combination RFC 0015 §7 question 4 assumes
// cannot reach the numeric index.
var longNumericString = strings.Repeat("9", 300)

// typedAttributeProbes are the spans written for the measurement, one per type
// the probe attribute can arrive as. The span name identifies the probe in the
// assertions below.
var typedAttributeProbes = []struct {
	span string
	put  func(pcommon.Map)
}{
	{"int", func(a pcommon.Map) { a.PutInt(probeKey, 4096) }},
	{"double", func(a pcommon.Map) { a.PutDouble(probeKey, 0.001) }},
	{"bool", func(a pcommon.Map) { a.PutBool(probeKey, true) }},
	{"word", func(a pcommon.Map) { a.PutStr(probeKey, "GET") }},
	{"numeric-string", func(a pcommon.Map) { a.PutStr(probeKey, "3") }},
	{"long-numeric-string", func(a pcommon.Map) { a.PutStr(probeKey, longNumericString) }},
	{"string-true", func(a pcommon.Map) { a.PutStr(probeKey, "true") }},
	{"above-double-precision", func(a pcommon.Map) { a.PutInt(probeKey, doublePrecisionLimit+1) }},
}

// leafQuery builds a predicate over one of the probe's mapped fields, given the
// base path the representation stores the value at.
type leafQuery func(valueField string) query.Query

// representation is one of the two places Jaeger puts an attribute. RFC 0015
// expects the elevated object to be the weaker of the two, because the nested
// array records each value's OTLP type beside it and the elevated object has
// nowhere to put such a discriminator. Every assertion below holds equally for
// both, and none of them reads the discriminator: with `coerce: false` the
// numeric sub-field already says whether a value arrived as a number, which is
// what the discriminator would have been consulted for.
type representation struct {
	name string
	// allTagsAsFields drives elasticsearch.tags_as_fields.all, which is what
	// decides whether an attribute is elevated out of the nested array.
	allTagsAsFields bool
	// wrap applies a leaf predicate to the probe attribute alone.
	wrap func(leafQuery) query.Query
	// ignoredField names what the _ignored metadata field records when a probe
	// value will not coerce into the numeric sub-field.
	ignoredField string
}

var representations = []representation{
	{
		name:            "nested",
		allTagsAsFields: false,
		wrap: func(leaf leafQuery) query.Query {
			// The nested query is what keeps the key and the value clauses on the
			// same array element rather than on two different attributes.
			return query.NewNestedQuery("tags", query.NewBoolQuery().Must(
				query.NewTermQuery("tags.key", probeKey),
				leaf("tags.value"),
			))
		},
		ignoredField: "tags.value.number",
	},
	{
		name:            "elevated",
		allTagsAsFields: true,
		wrap: func(leaf leafQuery) query.Query {
			return leaf("tag." + probeKey)
		},
		ignoredField: "tag." + probeKey + ".number",
	},
}

func greaterThan(threshold any) leafQuery {
	return func(valueField string) query.Query {
		return query.NewRangeQuery(valueField + ".number").Gt(threshold)
	}
}

func numberEquals(value any) leafQuery {
	return func(valueField string) query.Query {
		return query.NewTermQuery(valueField+".number", value)
	}
}

func booleanEquals(value bool) leafQuery {
	return func(valueField string) query.Query {
		return query.NewTermQuery(valueField+".boolean", value)
	}
}

func keywordEquals(value string) leafQuery {
	return func(valueField string) query.Query {
		return query.NewTermQuery(valueField, value)
	}
}

// TestElasticsearchStorage_TypedAttributeIndexing measures RFC 0015's Option A
// against a live backend, in both attribute representations. It runs in the
// existing ES/OS matrix job, so the answers cover Elasticsearch 7-9 and
// OpenSearch 1-3 rather than the one version a manual probe reaches.
func TestElasticsearchStorage_TypedAttributeIndexing(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))

	for _, rep := range representations {
		t.Run(rep.name, func(t *testing.T) {
			s := newTypedAttributeFixture(t, rep)
			s.assertOrdering(t)
			s.assertDoublePrecision(t)
			s.assertNoBooleanSubField(t)
			s.assertIgnoredIsQueryable(t)
			s.assertKeywordUnchanged(t)
		})
	}
}

// typedAttributeFixture is a backend holding one span per probe, with the numeric
// sub-field mapped.
type typedAttributeFixture struct {
	rep      representation
	searcher esclient.SearchClient
}

func newTypedAttributeFixture(t *testing.T, rep representation) *typedAttributeFixture {
	// The gate has to be on before the factory installs the index template, since
	// that is what puts the sub-fields in the mapping. The suite's setup then
	// deletes the existing indices, so every span below lands in an index created
	// from the new template.
	original := esclient.TypedAttributeIndexingGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(esclient.TypedAttributeIndexingGate.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(esclient.TypedAttributeIndexingGate.ID(), original))
	})

	s := &ESStorageIntegration{}
	s.initializeES(t, rep.allTagsAsFields)
	// Leave no gate-on template behind for the tests that run after this one.
	t.Cleanup(func() { s.client.cleanTemplates(t, indexPrefix) })

	f := &typedAttributeFixture{
		rep:      rep,
		searcher: esclient.SearchClient{Client: s.client.client},
	}

	trace := ptrace.NewTraces()
	rs := trace.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "typed_attribute_service")
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	start := pcommon.NewTimestampFromTime(time.Now().Truncate(time.Microsecond))
	for i, probe := range typedAttributeProbes {
		span := spans.AppendEmpty()
		span.SetName(probe.span)
		span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 77, 0, 0, 0, 0, 0, 0, 0, 88})
		span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, byte(i + 1)})
		span.SetStartTimestamp(start)
		span.SetEndTimestamp(start)
		probe.put(span.Attributes())
	}
	require.NoError(t, s.TraceWriter.WriteTraces(context.Background(), trace))

	// Every document was accepted: nothing about adding a sub-field may cost a
	// span, which is RFC 0015's R4. Search visibility waits on the refresh, so
	// this polls rather than asserting once.
	require.Eventually(t, func() bool {
		return len(f.search(t, nil)) == len(typedAttributeProbes)
	}, 30*time.Second, 200*time.Millisecond,
		"every probe span must be indexed; a rejected document would be a mapping-stability failure")

	return f
}

// search returns the names of the spans matching q, or every span when q is nil.
func (f *typedAttributeFixture) search(t *testing.T, q query.Query) []string {
	resp, err := f.searcher.Search(context.Background(), []string{probeIndex}, esclient.SearchRequest{
		Query: q,
		Size:  100,
	})
	require.NoError(t, err)
	names := make([]string, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var span struct {
			OperationName string `json:"operationName"`
		}
		require.NoError(t, json.Unmarshal(hit.Source, &span))
		names = append(names, span.OperationName)
	}
	slices.Sort(names)
	return names
}

// matches returns the span names matching a predicate on the probe attribute.
func (f *typedAttributeFixture) matches(t *testing.T, leaf leafQuery) []string {
	return f.search(t, f.rep.wrap(leaf))
}

// assertOrdering is the whole point of the change: a range predicate on an
// arbitrary attribute (RFC 0015's R1), answered numerically rather than
// lexicographically. It is also where `coerce: false` earns its place — without
// it a string that merely looks like a number is indexed as one, and the answer
// silently includes values the caller did not ask for (R2).
func (f *typedAttributeFixture) assertOrdering(t *testing.T) {
	assert.Equal(t,
		[]string{"above-double-precision", "int"},
		f.matches(t, greaterThan(2)),
		"only values that arrived as numbers greater than 2 may match")

	// Lexicographically "4096" < "9", so a keyword range would order these wrongly.
	// This is the comparison the RFC opens with.
	assert.Equal(t,
		[]string{"above-double-precision", "int"},
		f.matches(t, greaterThan(1000)),
		"4096 must compare as a number, not as the string \"4096\"")

	assert.Equal(t,
		[]string{"above-double-precision", "double", "int"},
		f.matches(t, greaterThan(0.0001)),
		"a float attribute must be ordered too")
}

// assertDoublePrecision answers RFC 0015 §7 question 2: a single `double`
// sub-field is proposed for field economy, and the cost is that an integer above
// 2^53 is rounded. The keyword still holds the value exactly, so nothing is lost
// from the stored span — only the ordered predicate is approximate up there.
func (f *typedAttributeFixture) assertDoublePrecision(t *testing.T) {
	assert.Equal(t,
		[]string{"above-double-precision"},
		f.matches(t, numberEquals(doublePrecisionLimit)),
		"an integer above 2^53 is indexed as its double neighbour")
	assert.Equal(t,
		[]string{"above-double-precision"},
		f.matches(t, keywordEquals("9007199254740993")),
		"the keyword still carries the exact value, so eq is unaffected")
}

// assertNoBooleanSubField answers RFC 0015 §7 question 7, which asked whether a
// boolean sub-field is worth its field. It is not mapped, so a predicate naming
// it matches nothing. Two things put it out of reach rather than merely making it
// a poor trade: OpenSearch rejects `ignore_malformed` on a boolean mapper, and
// without that parameter a value that is not a boolean costs the whole document.
// TestRenderSpanTemplateTypedAttributesEnabled asserts the mapping's absence;
// this asserts the query behaving as if the field does not exist.
func (f *typedAttributeFixture) assertNoBooleanSubField(t *testing.T) {
	assert.Empty(t, f.matches(t, booleanEquals(true)),
		"no boolean sub-field is mapped, so nothing is indexed under it")

	// Nothing is lost by its absence: the keyword renders a JSON boolean as the
	// term "true", so equality — the only operator a boolean gets — still works.
	assert.Contains(t, f.matches(t, keywordEquals("true")), "bool")
}

// assertIgnoredIsQueryable answers RFC 0015 §7 question 5: a value skipped by
// ignore_malformed is recorded in the _ignored metadata field, so "which spans
// carried a value the numeric sub-field could not take" is answerable rather
// than invisible.
//
// _ignored names the mapped field, which is why this is only useful on the
// elevated side. There the field is per key, so the answer names the attribute;
// on the nested side every attribute shares one `tags.value`, so the entry says
// only that some attribute on the span did not coerce.
func (f *typedAttributeFixture) assertIgnoredIsQueryable(t *testing.T) {
	ignored := f.search(t, query.NewTermQuery("_ignored", f.rep.ignoredField))
	assert.NotEmpty(t, ignored, "_ignored must record the skipped sub-field")
	assert.NotContains(t, ignored, "int", "a value that coerced cleanly is not recorded")

	if f.rep.name != "elevated" {
		return
	}
	// Every probe that did not arrive as a number, the boolean included — a JSON
	// boolean does not coerce into a double field either.
	assert.Equal(t,
		[]string{"bool", "long-numeric-string", "numeric-string", "string-true", "word"},
		ignored,
		"on the elevated side _ignored names the key, so it attributes the skip")
}

// assertKeywordUnchanged is the compatibility half of the change. The sub-field
// is additive, so exact match and the ignore_above bound must behave exactly as
// they did before, or every existing tag query changes meaning.
func (f *typedAttributeFixture) assertKeywordUnchanged(t *testing.T) {
	assert.Equal(t, []string{"word"}, f.matches(t, keywordEquals("GET")))
	assert.Equal(t, []string{"numeric-string"}, f.matches(t, keywordEquals("3")))

	// The keyword mapping renders a JSON boolean as the term "true", so it has
	// always conflated a boolean with the string that spells it. This is existing
	// behavior, unchanged by the sub-field, and it is also why §7 question 7's
	// premise does not hold: the ambiguity it wanted a boolean sub-field to
	// resolve lives in the keyword.
	assert.Equal(t, []string{"bool", "string-true"}, f.matches(t, keywordEquals("true")),
		"the keyword does not distinguish a boolean from the string \"true\"")

	// RFC 0015 §7 question 4: the 300-character value exceeds ignore_above, so the
	// keyword never indexed it. What the question assumed is that its length also
	// keeps it out of the numeric index; `coerce: false` is what actually does
	// that, and assertOrdering above is where it shows.
	assert.Empty(t, f.matches(t, keywordEquals(longNumericString)),
		"a value longer than ignore_above is stored but not indexed as a keyword")
}

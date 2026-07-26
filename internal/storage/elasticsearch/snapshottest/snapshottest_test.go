// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package snapshottest

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestRecorderCapturesJSONBody(t *testing.T) {
	rec := NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	sentBody := []byte(`{"size":0,"query":{"term":{"a":"b"}}}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/jaeger-span/_search?rest_total_hits_as_int=true", bytes.NewReader(sentBody))
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	requests := rec.Requests()
	require.Len(t, requests, 1)
	got := requests[0]
	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/jaeger-span/_search", got.Path)
	assert.Equal(t, url.Values{"rest_total_hits_as_int": {"true"}}, got.Query)
	// The body is recorded verbatim, in the order it was sent.
	assert.Equal(t, sentBody, got.Body)

	// Marshal parses and canonicalizes: object keys are sorted (query before size).
	snapshot := Marshal(t, requests)
	assert.Contains(t, snapshot, `"path": "/jaeger-span/_search"`)
	assert.Less(t, strings.Index(snapshot, `"query"`), strings.Index(snapshot, `"size"`))
}

func TestRecorderCapturesNDJSON(t *testing.T) {
	rec := NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	sentBody := []byte(`{"index":{"_index":"jaeger-span","_id":"1"}}` + "\n" + `{"traceID":"abc"}` + "\n")
	req, err := http.NewRequest(http.MethodPost, server.URL+"/_bulk", bytes.NewReader(sentBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	requests := rec.Requests()
	require.Len(t, requests, 1)
	got := requests[0]
	assert.Equal(t, "/_bulk", got.Path)
	// The Content-Type is recorded so NDJSON bodies can be validated against it.
	assert.Equal(t, "application/x-ndjson", got.ContentType)
	// The newline-delimited body is recorded verbatim.
	assert.Equal(t, sentBody, got.Body)

	// Marshal splits it into one canonicalized document per line.
	snapshot := Marshal(t, requests)
	assert.Contains(t, snapshot, `"ndjson"`)
}

func TestRecorderCapturesEmptyBody(t *testing.T) {
	rec := NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	req, err := http.NewRequest(http.MethodHead, server.URL+"/jaeger-span-read", http.NoBody)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	requests := rec.Requests()
	require.Len(t, requests, 1)
	assert.Empty(t, requests[0].Body)
	// Marshal omits an empty body and empty query.
	snapshot := Marshal(t, requests)
	assert.NotContains(t, snapshot, "body")
	assert.NotContains(t, snapshot, "query")

	rec.Reset()
	assert.Empty(t, rec.Requests())
}

func TestMarshalSortsRepeatedQueryValues(t *testing.T) {
	// The same repeated param sent in different orders yields the same snapshot.
	descending := []CapturedRequest{{Method: http.MethodGet, Path: "/x", Query: url.Values{"f": {"b", "a"}}}}
	ascending := []CapturedRequest{{Method: http.MethodGet, Path: "/x", Query: url.Values{"f": {"a", "b"}}}}
	assert.Equal(t, Marshal(t, ascending), Marshal(t, descending))
	assert.Less(t, strings.Index(Marshal(t, descending), `"a"`), strings.Index(Marshal(t, descending), `"b"`))
}

func TestParseVariant(t *testing.T) {
	const stem = "get_services"
	tests := []struct {
		name        string
		allVersions bool
		ranges      []backendRange
		ok          bool
	}{
		{name: "get_services.json", allVersions: true, ok: true},
		{name: "get_services.es7.json", ranges: []backendRange{{"es", 7, 7}}, ok: true},
		{name: "get_services.es7-8.json", ranges: []backendRange{{"es", 7, 8}}, ok: true},
		{name: "get_services.os1-3.json", ranges: []backendRange{{"os", 1, 3}}, ok: true},
		{name: "get_services.es7-9.os1-3.json", ranges: []backendRange{{"es", 7, 9}, {"os", 1, 3}}, ok: true},
		{name: "get_operations.es7.json", ok: false},    // different subject
		{name: "get_services.es.json", ok: false},       // missing major
		{name: "get_services.es7..os1.json", ok: false}, // empty range token
		{name: "get_services.txt", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := parseVariant(stem, tt.name)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.allVersions, v.allVersions)
				assert.Equal(t, tt.ranges, v.ranges)
			}
		})
	}
}

func TestBackendKey(t *testing.T) {
	tests := []struct {
		version es.BackendVersion
		backend string
		major   int
	}{
		{es.ElasticV7, "es", 7},
		{es.ElasticV9, "es", 9},
		{es.OpenSearch1, "os", 1},
		{es.OpenSearch3, "os", 3},
	}
	for _, tt := range tests {
		backend, major := backendKey(tt.version)
		assert.Equal(t, tt.backend, backend, tt.version.String())
		assert.Equal(t, tt.major, major, tt.version.String())
	}
}

// TestAssertByVersion_RegenerateCollapsesRanges exercises the full
// regenerate → assert round trip and verifies range collapsing.
func TestAssertByVersion_RegenerateCollapsesRanges(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "get_services")
	// Contrived content (not a real wire difference): ES7 emits a distinct
	// payload while every other version emits an identical one, so regeneration
	// must produce two files — es7.json plus a merged es8-9.os1-3.json —
	// exercising the range-collapse logic.
	content := map[es.BackendVersion]string{
		es.ElasticV7:   "ES7",
		es.ElasticV8:   "REST",
		es.ElasticV9:   "REST",
		es.OpenSearch1: "REST",
		es.OpenSearch2: "REST",
		es.OpenSearch3: "REST",
	}

	withRegenerate(t, true, func() {
		AssertByVersion(t, prefix, content)
	})

	files := listJSON(t, dir)
	assert.ElementsMatch(t, []string{
		"get_services.es7.json", "get_services.es8-9.os1-3.json",
	}, files, "byte-identical backends merge into one file")

	got, err := os.ReadFile(filepath.Join(dir, "get_services.es8-9.os1-3.json"))
	require.NoError(t, err)
	assert.Equal(t, "REST\n", string(got))

	// Assert mode passes against the freshly generated snapshots.
	withRegenerate(t, false, func() {
		AssertByVersion(t, prefix, content)
	})
}

// TestAssertByVersion_RegenerateBareWhenAllIdentical checks that content shared
// by every supported version collapses to the bare <subject>.json.
func TestAssertByVersion_RegenerateBareWhenAllIdentical(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "get_services")
	content := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		content[version] = "SAME"
	}

	withRegenerate(t, true, func() {
		AssertByVersion(t, prefix, content)
	})

	assert.ElementsMatch(t, []string{"get_services.json"}, listJSON(t, dir),
		"one wire format for all versions collapses to the bare file")
}

func TestAssertByVersion_RegeneratePrunesStaleAndIsSubjectScoped(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "get_services")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es8.json"), []byte("OLD\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_operations.es7.json"), []byte("other\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("unrelated"), 0o644))

	withRegenerate(t, true, func() {
		AssertByVersion(t, prefix, map[es.BackendVersion]string{es.ElasticV7: "NEW"})
	})

	files := listJSON(t, dir)
	assert.ElementsMatch(t, []string{"get_services.es7.json", "get_operations.es7.json"}, files,
		"stale get_services.es8.json pruned; other subjects untouched")
	_, err := os.Stat(filepath.Join(dir, "keep.txt"))
	assert.NoError(t, err, "unrelated files untouched")
}

func TestAssert(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "alias_exists")
	withRegenerate(t, true, func() {
		Assert(t, prefix, "SAME")
	})
	assert.ElementsMatch(t, []string{"alias_exists.json"}, listJSON(t, dir))

	// The bare snapshot resolves for every version.
	name, ok := resolveSnapshot(t, dir, "alias_exists", "os", 2)
	assert.True(t, ok)
	assert.Equal(t, "alias_exists.json", name)

	withRegenerate(t, false, func() {
		Assert(t, prefix, "SAME")
	})
}

func TestAssertPrunesAndReportsOrphanVariants(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "alias_exists")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alias_exists.json"), []byte("SAME\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alias_exists.es7.json"), []byte("stale\n"), 0o644))

	// Assert mode flags the stray per-version file committed beside the bare one.
	tb := &recordingTB{TB: t}
	Assert(tb, prefix, "SAME")
	require.Len(t, tb.errors, 1)
	assert.Contains(t, tb.errors[0], "orphan snapshot")

	// Regeneration prunes it, leaving only the bare file.
	withRegenerate(t, true, func() { Assert(t, prefix, "SAME") })
	assert.ElementsMatch(t, []string{"alias_exists.json"}, listJSON(t, dir))
}

func TestFindOrphans(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"get_services.es7.json", "get_services.es7-9.json", "get_services.os1-3.json",
		"get_services.os5.json", "get_operations.es7.json", "readme.md",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644))
	}
	used := map[string]bool{
		"get_services.es7.json": true, "get_services.es7-9.json": true, "get_services.os1-3.json": true,
	}
	// os5 is an unclaimed get_services snapshot; get_operations/readme belong to other subjects.
	assert.Equal(t, []string{"get_services.os5.json"}, findOrphans(t, dir, "get_services", used))

	used["get_services.os5.json"] = true
	assert.Empty(t, findOrphans(t, dir, "get_services", used))
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestRecorderBodyReadError(t *testing.T) {
	rec := NewRecorder(func(http.ResponseWriter, *http.Request) {
		t.Fatal("respond must not run when the body cannot be read")
	})
	w := httptest.NewRecorder()
	rec.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/_bulk", errReader{}))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, rec.Requests(), "a request that could not be read is not recorded")
}

func TestRecorderMarshalAndAssert(t *testing.T) {
	rec := NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/_cluster/health", http.NoBody)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	// The method form is shorthand for the package-level Marshal.
	assert.Equal(t, Marshal(t, rec.Requests()), rec.Marshal(t))

	dir := t.TempDir()
	prefix := filepath.Join(dir, "cluster_health")
	withRegenerate(t, true, func() { rec.Assert(t, prefix) })
	assert.ElementsMatch(t, []string{"cluster_health.json"}, listJSON(t, dir))
	withRegenerate(t, false, func() { rec.Assert(t, prefix) })
}

func TestMarshalMultipleRequests(t *testing.T) {
	requests := []CapturedRequest{
		{Method: http.MethodGet, Path: "/a"},
		{Method: http.MethodGet, Path: "/b"},
	}
	// Multiple requests render as a JSON array so the ordering is preserved.
	snapshot := Marshal(t, requests)
	assert.True(t, strings.HasPrefix(snapshot, "["), snapshot)
	assert.Less(t, strings.Index(snapshot, "/a"), strings.Index(snapshot, "/b"))
}

func TestParseNDJSONMalformed(t *testing.T) {
	_, err := parseNDJSON([]byte("{not json}"))
	assert.Error(t, err)
}

// TestParseNDJSONRejectsBlankLines covers the blank line a backend would reject.
// Skipping it instead would let "{a}\n\n{b}" snapshot identically to "{a}\n{b}",
// hiding the difference from both the snapshot and the reviewer.
func TestParseNDJSONRejectsBlankLines(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "leading", body: "\n" + `{"a":1}`},
		{name: "interior", body: `{"a":1}` + "\n\n" + `{"b":2}`},
		{name: "whitespace only", body: `{"a":1}` + "\n  \n" + `{"b":2}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseNDJSON([]byte(tt.body))
			require.ErrorContains(t, err, "must not contain empty lines")
		})
	}

	docs, err := parseNDJSON([]byte(`{"a":1}` + "\n" + `{"b":2}`))
	require.NoError(t, err)
	assert.Len(t, docs, 2)
}

func TestIsNDJSONEndpoint(t *testing.T) {
	assert.True(t, isNDJSONEndpoint("/_bulk"))
	assert.True(t, isNDJSONEndpoint("/_msearch"))
	assert.True(t, isNDJSONEndpoint("/jaeger-span-read/_msearch"))
	assert.False(t, isNDJSONEndpoint("/_search"))
	assert.False(t, isNDJSONEndpoint("/jaeger-span/_doc"))
}

// TestValidateNDJSON covers the wire-level rules the canonicalized snapshot cannot
// express: the accepted media types, and a body ending in exactly one newline.
func TestValidateNDJSON(t *testing.T) {
	const body = `{"index":{}}` + "\n" + `{"a":1}` + "\n"
	for _, tt := range []struct {
		name        string
		contentType string
		body        string
		wantErr     string
	}{
		{name: "x-ndjson", contentType: "application/x-ndjson", body: body},
		{name: "json is also accepted", contentType: "application/json", body: body},
		{name: "media type with charset", contentType: "application/x-ndjson; charset=utf-8", body: body},
		{name: "missing content type", contentType: "", body: body, wantErr: "is not one of"},
		{name: "wrong content type", contentType: "text/plain", body: body, wantErr: "is not one of"},
		{name: "empty body", contentType: "application/x-ndjson", body: "", wantErr: "must not be empty"},
		{name: "newline only", contentType: "application/x-ndjson", body: "\n", wantErr: "at least one JSON document"},
		{name: "whitespace only", contentType: "application/x-ndjson", body: "  \n", wantErr: "at least one JSON document"},
		{
			name: "no trailing newline", contentType: "application/x-ndjson",
			body: `{"index":{}}` + "\n" + `{"a":1}`, wantErr: "must end with a newline",
		},
		{
			name: "extra trailing newline", contentType: "application/x-ndjson",
			body: body + "\n", wantErr: "must end with exactly one newline",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNDJSON(tt.contentType, []byte(tt.body))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSplitPrefixDefaultsDir(t *testing.T) {
	dir, stem := splitPrefix("version")
	assert.Equal(t, ".", dir)
	assert.Equal(t, "version", stem)
}

func TestResolveSnapshotNotFound(t *testing.T) {
	name, ok := resolveSnapshot(t, t.TempDir(), "get_services", "es", 6)
	assert.False(t, ok)
	assert.Empty(t, name)
}

func TestResolveSnapshotRejectsOverlappingRanges(t *testing.T) {
	dir := t.TempDir()
	// Two hand-committed variants both claim es8; regeneration never emits this.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es7-8.json"), []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es8-9.json"), []byte("y\n"), 0o644))

	tb := &recordingTB{TB: t}
	resolveSnapshot(tb, dir, "get_services", "es", 8)
	require.Len(t, tb.errors, 1)
	assert.Contains(t, tb.errors[0], "must not overlap")

	// A non-overlapping major still resolves cleanly.
	tb = &recordingTB{TB: t}
	name, ok := resolveSnapshot(tb, dir, "get_services", "es", 7)
	assert.True(t, ok)
	assert.Equal(t, "get_services.es7-8.json", name)
	assert.Empty(t, tb.errors)
}

// recordingTB captures Error/Errorf instead of failing, so tests can exercise the
// harness's own error-reporting branches. The embedded testing.TB (a real *T)
// satisfies the interface and backs every other method.
type recordingTB struct {
	testing.TB
	errors []string
}

func (r *recordingTB) Error(args ...any) { r.errors = append(r.errors, fmt.Sprint(args...)) }
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestAssertByVersionReportsMissingSnapshot(t *testing.T) {
	tb := &recordingTB{TB: t}
	// The directory does not exist yet (first run for the subject); the harness
	// treats it as empty and reports the actionable error instead of failing on
	// the missing directory.
	AssertByVersion(tb, filepath.Join(t.TempDir(), "nonexistent", "get_services"),
		map[es.BackendVersion]string{es.ElasticV7: "REST"})
	require.Len(t, tb.errors, 1)
	assert.Contains(t, tb.errors[0], "no snapshot file")
}

func TestAssertNoOrphansReports(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es7.json"), []byte("x\n"), 0o644))
	tb := &recordingTB{TB: t}
	assertNoOrphans(tb, dir, "get_services", map[string]bool{})
	require.Len(t, tb.errors, 1)
	assert.Contains(t, tb.errors[0], "orphan snapshot")
}

func withRegenerate(t *testing.T, value bool, fn func()) {
	t.Helper()
	prev := Regenerate
	Regenerate = value
	defer func() { Regenerate = prev }()
	fn()
}

func listJSON(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

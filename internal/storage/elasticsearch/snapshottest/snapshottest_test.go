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
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	requests := rec.Requests()
	require.Len(t, requests, 1)
	got := requests[0]
	assert.Equal(t, "/_bulk", got.Path)
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
		backend     string
		lo, hi      int
		ok          bool
	}{
		{name: "get_services.json", allVersions: true, ok: true},
		{name: "get_services.es6.json", backend: "es", lo: 6, hi: 6, ok: true},
		{name: "get_services.es6-7.json", backend: "es", lo: 6, hi: 7, ok: true},
		{name: "get_services.os1-3.json", backend: "os", lo: 1, hi: 3, ok: true},
		{name: "get_operations.es6.json", ok: false}, // different subject
		{name: "get_services.es.json", ok: false},    // missing major
		{name: "get_services.txt", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := parseVariant(stem, tt.name)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.allVersions, v.allVersions)
				assert.Equal(t, tt.backend, v.backend)
				assert.Equal(t, tt.lo, v.lo)
				assert.Equal(t, tt.hi, v.hi)
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
		{es.ElasticV6, "es", 6},
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
	// ES6 differs; ES7/8/9 identical; OS1/2/3 identical (but distinct from ES).
	content := map[es.BackendVersion]string{
		es.ElasticV6:   "ES6",
		es.ElasticV7:   "REST",
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
		"get_services.es6.json", "get_services.es7-9.json", "get_services.os1-3.json",
	}, files, "backends stay separate even when byte-identical")

	got, err := os.ReadFile(filepath.Join(dir, "get_services.es7-9.json"))
	require.NoError(t, err)
	assert.Equal(t, "REST\n", string(got))

	// Assert mode passes against the freshly generated snapshots.
	withRegenerate(t, false, func() {
		AssertByVersion(t, prefix, content)
	})
}

func TestAssertByVersion_RegeneratePrunesStaleAndIsSubjectScoped(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "get_services")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es8.json"), []byte("OLD\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_operations.es6.json"), []byte("other\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("unrelated"), 0o644))

	withRegenerate(t, true, func() {
		AssertByVersion(t, prefix, map[es.BackendVersion]string{es.ElasticV7: "NEW"})
	})

	files := listJSON(t, dir)
	assert.ElementsMatch(t, []string{"get_services.es7.json", "get_operations.es6.json"}, files,
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

func TestFindOrphans(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"get_services.es6.json", "get_services.es7-9.json", "get_services.os1-3.json",
		"get_services.os5.json", "get_operations.es6.json", "readme.md",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644))
	}
	used := map[string]bool{
		"get_services.es6.json": true, "get_services.es7-9.json": true, "get_services.os1-3.json": true,
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
	// The leading blank line is skipped; the malformed line surfaces the error.
	_, err := parseNDJSON([]byte("\n{not json}"))
	assert.Error(t, err)
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "get_services.es6.json"), []byte("x\n"), 0o644))
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

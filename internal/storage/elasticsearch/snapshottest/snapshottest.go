// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package snapshottest provides a request-wire-format snapshot harness for the
// Elasticsearch/OpenSearch clients, as described in RFC 0006 (Unified
// Elasticsearch/OpenSearch Client), milestone M1.
//
// The harness records the exact HTTP request(s) an operation emits — method,
// path, sorted query params, and canonicalized JSON body (or NDJSON for
// _bulk/_msearch) — and compares them against committed snapshot files.
//
// Snapshot files follow the §7.3 fixture taxonomy:
//
//	testdata/<subject>[.<backend><range>].json
//
// A request that is identical for all backends and versions is stored as the
// bare "testdata/<subject>.json". A request that varies by backend/version is
// stored per variant as "testdata/<subject>.<backend><range>.json", where
// <backend> is "es" or "os" and <range> is a single major ("8") or an inclusive,
// non-overlapping major range ("6-7") shared by consecutive versions that emit
// byte-identical output. <subject> may nest with "/" to group a family.
//
// Callers pass the path stem (dir + subject, e.g. "testdata/get_services"); the
// harness appends the ".json" / ".<backend><range>.json" tail.
//
// Snapshot files are regenerated (and range-collapsed) by running the tests with
// the REGENERATE_SNAPSHOTS environment variable set:
//
//	REGENERATE_SNAPSHOTS=true go test ./...
package snapshottest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

// Regenerate reports whether snapshot files should be rewritten from the observed
// requests instead of being asserted against. It is controlled by the
// REGENERATE_SNAPSHOTS environment variable.
var Regenerate = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

// CapturedRequest is a faithful record of a single HTTP request as received: the
// method, path, parsed query, and the raw body bytes exactly as sent. Turning it
// into a canonical, diffable snapshot happens in Marshal, not here.
type CapturedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

// Recorder is an http.Handler that records every request it receives and
// delegates the response to a user-supplied function.
type Recorder struct {
	respond func(http.ResponseWriter, *http.Request)

	mu       sync.Mutex
	requests []CapturedRequest
}

// NewRecorder returns a Recorder that serves responses via respond. The respond
// function is where the test returns whatever canned payload the client needs to
// parse (version JSON, an empty search result, a bulk response, etc.).
func NewRecorder(respond func(http.ResponseWriter, *http.Request)) *Recorder {
	return &Recorder{respond: respond}
}

func (rec *Recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// Fail fast: a partial read must not be recorded, or it could silently
		// pass a snapshot assertion against a truncated wire format. Erroring the
		// response surfaces as a client-side failure the test's require.NoError
		// catches.
		http.Error(w, "snapshottest: reading request body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	captured := CapturedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   body,
	}
	if q := r.URL.Query(); len(q) > 0 {
		captured.Query = q
	}
	rec.mu.Lock()
	rec.requests = append(rec.requests, captured)
	rec.mu.Unlock()
	// Restore the body so respond can still read it (e.g. to branch a canned
	// response on the request payload).
	r.Body = io.NopCloser(bytes.NewReader(body))
	rec.respond(w, r)
}

// Requests returns the requests captured so far.
func (rec *Recorder) Requests() []CapturedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := make([]CapturedRequest, len(rec.requests))
	copy(out, rec.requests)
	return out
}

// Reset clears the captured requests so a Recorder can be reused across
// operations or backend versions.
func (rec *Recorder) Reset() {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.requests = nil
}

// snapshotRequest is the canonical, marshalable form of a CapturedRequest: the
// body parsed to sorted-key JSON (or, for a newline-delimited _bulk/_msearch
// body, to NDJSON documents). Marshaling sorts object keys, so the output is
// deterministic regardless of the order the client emitted fields in.
type snapshotRequest struct {
	Method string     `json:"method"`
	Path   string     `json:"path"`
	Query  url.Values `json:"query,omitempty"`
	// Body is a single JSON body, which may be any JSON value.
	Body any `json:"body,omitempty"`
	// NDJSON is the documents of a newline-delimited body, each a JSON object (an
	// action/metadata or document/query line).
	NDJSON []map[string]any `json:"ndjson,omitempty"`
}

// Marshal renders captured requests as canonical, indented JSON. A single request
// is rendered as an object; multiple requests as an array, so snapshots stay clean
// for the common one-request case.
func Marshal(t testing.TB, requests []CapturedRequest) string {
	t.Helper()
	snapshots := make([]snapshotRequest, len(requests))
	for i, r := range requests {
		snapshots[i] = toSnapshot(t, r)
	}
	var (
		out []byte
		err error
	)
	if len(snapshots) == 1 {
		out, err = json.MarshalIndent(snapshots[0], "", "  ")
	} else {
		out, err = json.MarshalIndent(snapshots, "", "  ")
	}
	require.NoError(t, err)
	return string(out)
}

func toSnapshot(t testing.TB, r CapturedRequest) snapshotRequest {
	t.Helper()
	s := snapshotRequest{Method: r.Method, Path: r.Path, Query: canonicalQuery(r.Query)}
	body := bytes.TrimRight(r.Body, "\n")
	if len(body) == 0 {
		return s
	}
	// A single JSON document parses whole; a newline-delimited body does not.
	var single any
	if err := json.Unmarshal(body, &single); err == nil {
		s.Body = single
		return s
	}
	docs, err := parseNDJSON(body)
	require.NoErrorf(t, err, "request body is neither JSON nor NDJSON: %s", body)
	s.NDJSON = docs
	return s
}

// canonicalQuery returns a copy of q with each value slice sorted, so a repeated
// query param yields a stable snapshot regardless of the order it was sent in.
// (JSON marshaling already sorts the keys.)
func canonicalQuery(q url.Values) url.Values {
	if len(q) == 0 {
		return nil
	}
	out := make(url.Values, len(q))
	for key, values := range q {
		sorted := slices.Clone(values)
		slices.Sort(sorted)
		out[key] = sorted
	}
	return out
}

func parseNDJSON(body []byte) ([]map[string]any, error) {
	lines := bytes.Split(body, []byte("\n"))
	docs := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(line, &doc); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// Assert compares content against the single snapshot "<prefix>.json", for a
// request that is identical for all backends and versions (§7.3). With
// REGENERATE_SNAPSHOTS=true it (re)writes the file.
func Assert(t testing.TB, prefix string, content string) {
	t.Helper()
	path := prefix + ".json"
	if Regenerate {
		require.NoError(t, os.MkdirAll(filepath.Dir(prefix), 0o755))
		writeSnapshot(t, path, content)
	}
	assertFileEquals(t, path, content, "all versions")
}

// AssertByVersion compares each version's content against the snapshot files
// "<prefix>.<backend><range>.json", keeping ES and OpenSearch backends in
// separate files. With REGENERATE_SNAPSHOTS=true it rewrites the files,
// collapsing byte-identical consecutive majors within a backend into an
// inclusive range and pruning stale files for this subject.
func AssertByVersion(t testing.TB, prefix string, contentByVersion map[es.BackendVersion]string) {
	t.Helper()
	require.NotEmpty(t, contentByVersion, "no content provided for %s", prefix)
	dir, stem := splitPrefix(prefix)
	if Regenerate {
		regenerateVersioned(t, dir, stem, contentByVersion)
	}
	used := map[string]bool{}
	for version, content := range contentByVersion {
		backend, major := backendKey(version)
		name, ok := resolveSnapshot(t, dir, stem, backend, major)
		if !ok {
			t.Errorf("no snapshot file for %s in %s covering %s%d; run REGENERATE_SNAPSHOTS=true to create it",
				version, dir, backend, major)
			continue
		}
		used[name] = true
		assertFileEquals(t, filepath.Join(dir, name), content, version.String())
	}
	assertNoOrphans(t, dir, stem, used)
}

// splitPrefix separates a "dir/subject" path stem into its directory and subject
// components, defaulting the directory to "." when the stem has no directory.
func splitPrefix(prefix string) (dir, stem string) {
	dir, stem = filepath.Split(prefix)
	if dir == "" {
		dir = "."
	}
	return dir, stem
}

// assertNoOrphans fails if a snapshot file for this subject is never claimed by any
// tested version — a committed snapshot that no version resolves to is dead weight
// and a sign the matrix drifted. Regeneration prunes such files, so this fires
// only when a stale snapshot is committed by hand.
func assertNoOrphans(t testing.TB, dir, stem string, used map[string]bool) {
	t.Helper()
	for _, name := range findOrphans(t, dir, stem, used) {
		t.Errorf("orphan snapshot %q in %s is never loaded by any tested version", name, dir)
	}
}

// findOrphans returns this subject's snapshot files in dir not present in used,
// sorted.
func findOrphans(t testing.TB, dir, stem string, used map[string]bool) []string {
	t.Helper()
	var orphans []string
	for _, name := range subjectSnapshots(t, dir, stem) {
		if !used[name] {
			orphans = append(orphans, name)
		}
	}
	slices.Sort(orphans)
	return orphans
}

// subjectSnapshots lists the snapshot file names in dir that belong to stem.
func subjectSnapshots(t testing.TB, dir, stem string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "reading snapshot dir %s", dir)
	var names []string
	for _, e := range entries {
		if _, ok := parseVariant(stem, e.Name()); ok {
			names = append(names, e.Name())
		}
	}
	return names
}

var backendRangeRE = regexp.MustCompile(`^(es|os)(\d+)(?:-(\d+))?$`)

// snapshotVariant is a parsed snapshot filename for a subject stem: either the bare
// all-versions file or a per-backend major range.
type snapshotVariant struct {
	allVersions bool
	backend     string
	lo, hi      int
}

// parseVariant reports whether name is a snapshot file for stem and, if so, its
// variant. It accepts "<stem>.json" (all versions) and "<stem>.<backend><range>.json".
func parseVariant(stem, name string) (snapshotVariant, bool) {
	if name == stem+".json" {
		return snapshotVariant{allVersions: true}, true
	}
	rest, ok := strings.CutPrefix(name, stem+".")
	if !ok {
		return snapshotVariant{}, false
	}
	rest, ok = strings.CutSuffix(rest, ".json")
	if !ok {
		return snapshotVariant{}, false
	}
	m := backendRangeRE.FindStringSubmatch(rest)
	if m == nil {
		return snapshotVariant{}, false
	}
	v := snapshotVariant{backend: m[1]}
	v.lo, _ = strconv.Atoi(m[2])
	v.hi = v.lo
	if m[3] != "" {
		v.hi, _ = strconv.Atoi(m[3])
	}
	return v, true
}

// writeSnapshot stores content with exactly one trailing newline, so snapshots are
// byte-clean regardless of whether the content already ended in a newline.
func writeSnapshot(t testing.TB, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0o600))
}

func assertFileEquals(t testing.TB, path, content, label string) {
	t.Helper()
	want, err := os.ReadFile(path)
	require.NoError(t, err, "reading snapshot %s; run REGENERATE_SNAPSHOTS=true to create it", path)
	assert.Equal(t, strings.TrimRight(string(want), "\n"), strings.TrimRight(content, "\n"),
		"snapshot mismatch for %s (%s); run REGENERATE_SNAPSHOTS=true to update", label, path)
}

// backendKey maps a BackendVersion to its filename prefix and major number,
// e.g. ElasticV7 -> ("es", 7) and OpenSearch2 -> ("os", 2).
func backendKey(v es.BackendVersion) (string, int) {
	if v.IsOpenSearch() {
		return "os", int(v) - 100
	}
	return "es", int(v)
}

// resolveSnapshot finds the snapshot file for stem in dir that applies to
// (backend, major): the all-versions file if present, otherwise the unique variant
// whose inclusive range contains major.
func resolveSnapshot(t testing.TB, dir, stem, backend string, major int) (string, bool) {
	t.Helper()
	names := subjectSnapshots(t, dir, stem)
	for _, name := range names {
		if v, _ := parseVariant(stem, name); v.allVersions {
			return name, true
		}
	}
	for _, name := range names {
		if v, _ := parseVariant(stem, name); v.backend == backend && major >= v.lo && major <= v.hi {
			return name, true
		}
	}
	return "", false
}

type versionContent struct {
	major   int
	content string
}

func regenerateVersioned(t testing.TB, dir, stem string, contentByVersion map[es.BackendVersion]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))

	byBackend := map[string][]versionContent{}
	for version, content := range contentByVersion {
		backend, major := backendKey(version)
		byBackend[backend] = append(byBackend[backend], versionContent{major, content})
	}

	kept := map[string]bool{}
	for backend, entries := range byBackend {
		slices.SortFunc(entries, func(a, b versionContent) int { return a.major - b.major })
		for i := 0; i < len(entries); {
			// Extend the range while the next major is consecutive and identical.
			j := i
			for j+1 < len(entries) &&
				entries[j+1].major == entries[j].major+1 &&
				entries[j+1].content == entries[i].content {
				j++
			}
			name := variantFileName(stem, backend, entries[i].major, entries[j].major)
			writeSnapshot(t, filepath.Join(dir, name), entries[i].content)
			kept[name] = true
			i = j + 1
		}
	}
	pruneStaleSnapshots(t, dir, stem, kept)
}

func variantFileName(stem, backend string, lo, hi int) string {
	if hi > lo {
		return fmt.Sprintf("%s.%s%d-%d.json", stem, backend, lo, hi)
	}
	return fmt.Sprintf("%s.%s%d.json", stem, backend, lo)
}

// pruneStaleSnapshots removes this subject's snapshot files that were not written
// this run (e.g. left over after majors were collapsed into a range, or an old
// all-versions file). It never touches other subjects' or unrelated files.
func pruneStaleSnapshots(t testing.TB, dir, stem string, kept map[string]bool) {
	t.Helper()
	for _, name := range subjectSnapshots(t, dir, stem) {
		if !kept[name] {
			require.NoError(t, os.Remove(filepath.Join(dir, name)))
		}
	}
}

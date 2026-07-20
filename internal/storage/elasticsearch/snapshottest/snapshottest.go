// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package snapshottest provides a request-wire-format snapshot harness for the
// Elasticsearch/OpenSearch clients.
//
// The harness records the exact HTTP request(s) an operation emits — method,
// path, sorted query params, and canonicalized JSON body (or NDJSON for
// _bulk/_msearch) — and compares them against committed snapshot files.
//
// Snapshot files follow this fixture taxonomy:
//
//	testdata/<subject>[.<variants>].json
//
// There is exactly one file per distinct wire format. A request that is identical
// for all backends and versions is stored as the bare "testdata/<subject>.json".
// Otherwise each distinct wire format is stored as "testdata/<subject>.<variants>.json",
// where <variants> is a dot-separated list of the version ranges that emit it,
// e.g. "es7", "es8-9.os1-3". Each range is "<backend><lo>[-<hi>]" with <backend>
// "es" or "os" and an inclusive, consecutive major range. Backends are merged
// into one file when they emit byte-identical output, so there is no duplication.
// <subject> may nest with "/" to group a family.
//
// Callers pass the path stem (dir + subject, e.g. "testdata/get_services"); the
// harness appends the ".json" / ".<variants>.json" tail.
//
// Snapshot files are regenerated (and range-collapsed) by running the tests with
// the REGENERATE_SNAPSHOTS environment variable set:
//
//	REGENERATE_SNAPSHOTS=true go test ./...
package snapshottest

import (
	"bytes"
	"encoding/json"
	"errors"
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
	Method      string
	Path        string
	Query       url.Values
	Body        []byte
	ContentType string
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
		Method:      r.Method,
		Path:        r.URL.Path,
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
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

// Marshal renders the requests captured so far as a canonical snapshot string. It
// is shorthand for Marshal(t, rec.Requests()), and is the value fed to Assert or
// collected into an AssertByVersion map.
func (rec *Recorder) Marshal(t testing.TB) string {
	t.Helper()
	return Marshal(t, rec.Requests())
}

// Assert marshals the captured requests and compares them against the single
// all-versions snapshot "<prefix>.json" (see the package-level Assert). It is the
// shortcut for the common case of one recorder producing one snapshot; the
// by-version case marshals per version into a map for AssertByVersion instead.
func (rec *Recorder) Assert(t testing.TB, prefix string) {
	t.Helper()
	Assert(t, prefix, rec.Marshal(t))
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
	NDJSON      []map[string]any `json:"ndjson,omitempty"`
	ContentType string           `json:"contentType,omitempty"`
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
	s := snapshotRequest{Method: r.Method, Path: r.Path, Query: canonicalQuery(r.Query), ContentType: r.ContentType}
	body := r.Body

	if strings.HasSuffix(r.Path, "_bulk") || strings.HasSuffix(r.Path, "_msearch") {
		ct := strings.TrimSpace(strings.SplitN(r.ContentType, ";", 2)[0])
		require.Equal(t, "application/x-ndjson", ct, "invalid content type for path: %s", r.Path)
		trailingNewlines := len(body) - len(bytes.TrimRight(body, "\n"))
		require.Equal(t, 1, trailingNewlines, "body for %s must end with exactly one trailing newline", r.Path)
	}

	body = bytes.TrimRight(body, "\n")
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
			require.Failf(nil, "snapshottest", "NDJSON body has empty line")
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
// request that is identical for all backends and versions. With
// REGENERATE_SNAPSHOTS=true it (re)writes the file and prunes any stale
// per-version variants for this subject. Like AssertByVersion, it fails on an
// orphan snapshot — here, any per-version file committed alongside the bare one.
func Assert(t testing.TB, prefix string, content string) {
	t.Helper()
	dir, stem := splitPrefix(prefix)
	name := stem + ".json"
	used := map[string]bool{name: true}
	if Regenerate {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		writeSnapshot(t, filepath.Join(dir, name), content)
		pruneStaleSnapshots(t, dir, stem, used)
	}
	assertFileEquals(t, filepath.Join(dir, name), content, "all versions")
	assertNoOrphans(t, dir, stem, used)
}

// AssertByVersion compares each version's content against the snapshot files
// "<prefix>.<variants>.json". With REGENERATE_SNAPSHOTS=true it rewrites the
// files, storing one file per distinct wire format — merging byte-identical
// backends and collapsing consecutive majors into a range — and pruning stale
// files for this subject.
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
	if errors.Is(err, os.ErrNotExist) {
		// A missing directory is an empty snapshot set, so callers report the
		// actionable "run REGENERATE_SNAPSHOTS=true" message rather than a raw
		// read error on the first run for a subject.
		return nil
	}
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

// backendRange is one "<backend><lo>[-<hi>]" token: a backend ("es"/"os") and an
// inclusive major-version range.
type backendRange struct {
	backend string
	lo, hi  int
}

// contains reports whether (backend, major) falls in this range.
func (r backendRange) contains(backend string, major int) bool {
	return r.backend == backend && major >= r.lo && major <= r.hi
}

// snapshotVariant is a parsed snapshot filename for a subject stem: either the bare
// all-versions file or a list of the backend ranges the file covers.
type snapshotVariant struct {
	allVersions bool
	ranges      []backendRange
}

// parseVariant reports whether name is a snapshot file for stem and, if so, its
// variant. It accepts "<stem>.json" (all versions) and "<stem>.<variants>.json",
// where <variants> is a dot-separated list of "<backend><lo>[-<hi>]" ranges.
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
	var v snapshotVariant
	for token := range strings.SplitSeq(rest, ".") {
		m := backendRangeRE.FindStringSubmatch(token)
		if m == nil {
			return snapshotVariant{}, false
		}
		r := backendRange{backend: m[1]}
		r.lo, _ = strconv.Atoi(m[2])
		r.hi = r.lo
		if m[3] != "" {
			r.hi, _ = strconv.Atoi(m[3])
		}
		v.ranges = append(v.ranges, r)
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
// one of whose ranges contains it. It fails the test if more than one variant
// claims (backend, major) — ranges must not overlap — so a hand-committed
// overlap cannot be silently resolved to whichever file happens to sort first.
func resolveSnapshot(t testing.TB, dir, stem, backend string, major int) (string, bool) {
	t.Helper()
	names := subjectSnapshots(t, dir, stem)
	for _, name := range names {
		if v, _ := parseVariant(stem, name); v.allVersions {
			return name, true
		}
	}
	var matches []string
	for _, name := range names {
		v, _ := parseVariant(stem, name)
		for _, r := range v.ranges {
			if r.contains(backend, major) {
				matches = append(matches, name)
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	slices.Sort(matches)
	if len(matches) > 1 {
		t.Errorf("snapshot files %v in %s all claim %s%d; ranges must not overlap", matches, dir, backend, major)
	}
	return matches[0], true
}

func regenerateVersioned(t testing.TB, dir, stem string, contentByVersion map[es.BackendVersion]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// One file per distinct wire format: group the versions that emit each, so
	// byte-identical backends share a file instead of duplicating it.
	versionsByContent := map[string][]es.BackendVersion{}
	for version, content := range contentByVersion {
		versionsByContent[content] = append(versionsByContent[content], version)
	}

	kept := map[string]bool{}
	for content, versions := range versionsByContent {
		name := variantFileName(stem, versions)
		writeSnapshot(t, filepath.Join(dir, name), content)
		kept[name] = true
	}
	pruneStaleSnapshots(t, dir, stem, kept)
}

// variantFileName names the file covering exactly versions: the bare "<stem>.json"
// when they span every supported version, otherwise "<stem>.<variants>.json" with a
// dot-separated list of "<backend><lo>[-<hi>]" ranges (es before os, ascending),
// collapsing consecutive majors.
func variantFileName(stem string, versions []es.BackendVersion) string {
	if len(versions) == len(es.AllVersions) {
		return stem + ".json"
	}
	var esMajors, osMajors []int
	for _, v := range versions {
		if backend, major := backendKey(v); backend == "es" {
			esMajors = append(esMajors, major)
		} else {
			osMajors = append(osMajors, major)
		}
	}
	tokens := append(rangeTokens("es", esMajors), rangeTokens("os", osMajors)...)
	return stem + "." + strings.Join(tokens, ".") + ".json"
}

// rangeTokens renders majors as "<backend><lo>[-<hi>]" tokens, collapsing runs of
// consecutive majors into a single range.
func rangeTokens(backend string, majors []int) []string {
	slices.Sort(majors)
	var tokens []string
	for i := 0; i < len(majors); {
		j := i
		for j+1 < len(majors) && majors[j+1] == majors[j]+1 {
			j++
		}
		if majors[j] > majors[i] {
			tokens = append(tokens, fmt.Sprintf("%s%d-%d", backend, majors[i], majors[j]))
		} else {
			tokens = append(tokens, fmt.Sprintf("%s%d", backend, majors[i]))
		}
		i = j + 1
	}
	return tokens
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

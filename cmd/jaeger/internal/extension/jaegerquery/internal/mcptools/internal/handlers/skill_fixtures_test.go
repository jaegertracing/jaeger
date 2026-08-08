// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

const (
	skillFixtureDir = "testdata/skills"
	// Large enough that no fixture is truncated: these tests are about what the
	// tools report, not about the per-request cap.
	skillFixtureSpanLimit = 200
)

// skillFixture is the manifest recorded beside each captured trace: where it came
// from, whether the pattern is present, and the answer a skill should reach. Only
// the fields every fixture carries are declared; each test reads the parts of
// Expected that its own skill needs.
type skillFixture struct {
	Fixture        string          `json:"fixture"`
	Skill          string          `json:"skill"`
	PatternPresent bool            `json:"pattern_present"`
	TraceID        string          `json:"trace_id"`
	Expected       json.RawMessage `json:"expected"`
}

func (f skillFixture) expect(t *testing.T, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(f.Expected, target))
}

// loadSkillFixture reads a fixture and its manifest. The trace is OTLP JSON, the
// format Jaeger's v3 API returns, so it needs no conversion on the way in.
func loadSkillFixture(t *testing.T, name string) (ptrace.Traces, skillFixture) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(skillFixtureDir, name+".json"))
	require.NoError(t, err)
	traces, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(raw)
	require.NoError(t, err)

	rawManifest, err := os.ReadFile(filepath.Join(skillFixtureDir, name+".manifest.json"))
	require.NoError(t, err)
	var manifest skillFixture
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))

	return traces, manifest
}

// skillFixtureNames discovers fixtures from their manifests, so adding one does
// not also require editing a list here.
func skillFixtureNames(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(skillFixtureDir, "*.manifest.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(m), ".manifest.json"))
	}
	return names
}

func topologyOf(t *testing.T, name string) (types.GetTraceTopologyOutput, skillFixture) {
	t.Helper()
	traces, manifest := loadSkillFixture(t, name)
	h := &getTraceTopologyHandler{
		queryService:             newMockYieldingTraces(traces),
		maxSpanDetailsPerRequest: skillFixtureSpanLimit,
	}
	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{},
		types.GetTraceTopologyInput{TraceID: manifest.TraceID})
	require.NoError(t, err)
	return out, manifest
}

func errorsOf(t *testing.T, name string) (types.GetTraceErrorsOutput, skillFixture) {
	t.Helper()
	traces, manifest := loadSkillFixture(t, name)
	h := &getTraceErrorsHandler{
		queryService:             newMockYieldingTraces(traces),
		maxSpanDetailsPerRequest: skillFixtureSpanLimit,
	}
	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{},
		types.GetTraceErrorsInput{TraceID: manifest.TraceID})
	require.NoError(t, err)
	return out, manifest
}

// siblings returns the spans sharing a parent and operation name, keyed by the
// parent path. This is step 2 of detect-n-plus-one: "group child spans by
// operation name under each parent".
func siblings(spans []types.TopologySpan, parentSpanID, spanName string) []types.TopologySpan {
	var group []types.TopologySpan
	for _, s := range spans {
		idx := strings.LastIndex(s.Path, "/")
		if idx < 0 || s.SpanName != spanName {
			continue
		}
		parent := s.Path[:idx]
		if strings.HasSuffix(parent, parentSpanID) {
			group = append(group, s)
		}
	}
	return group
}

// serialityOf implements step 3 of detect-n-plus-one over what the tool actually
// returned: the sum of sibling durations against the window they occupy. Serial
// work sums to roughly the window; overlapping work sums to more.
func serialityOf(t *testing.T, group []types.TopologySpan) (sumUs, windowUs int64) {
	t.Helper()
	require.NotEmpty(t, group)

	var earliest, latest time.Time
	for i, s := range group {
		start, err := time.Parse(time.RFC3339Nano, s.StartTime)
		require.NoError(t, err)
		end := start.Add(time.Duration(s.DurationUs) * time.Microsecond)
		if i == 0 || start.Before(earliest) {
			earliest = start
		}
		if i == 0 || end.After(latest) {
			latest = end
		}
		sumUs += s.DurationUs
	}
	return sumUs, latest.Sub(earliest).Microseconds()
}

func TestSkillFixturesLoad(t *testing.T) {
	for _, name := range skillFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			traces, manifest := loadSkillFixture(t, name)
			assert.Positive(t, traces.SpanCount())
			assert.Contains(t, []string{"detect-n-plus-one", "error-root-cause"}, manifest.Skill)
			assert.NotEmpty(t, manifest.Expected)

			// Every span belongs to the trace the manifest names: a fixture that
			// mixed traces would make its expected answer meaningless.
			for _, rs := range traces.ResourceSpans().All() {
				for _, ss := range rs.ScopeSpans().All() {
					for _, span := range ss.Spans().All() {
						assert.Equal(t, manifest.TraceID, span.TraceID().String())
					}
				}
			}
		})
	}
}

// TestSkillFixtures_NPlusOneSeriality is the assertion detect-n-plus-one rests on:
// get_trace_topology must expose enough timing for the serial-vs-overlapped test
// to separate a real N+1 from a parallel fan-out. Duration similarity cannot —
// on these fixtures the fan-out is the more uniform of the two.
func TestSkillFixtures_NPlusOneSeriality(t *testing.T) {
	tests := []struct {
		fixture  string
		serial   bool
		minCount int
	}{
		{fixture: "n_plus_one_positive", serial: true, minCount: 10},
		{fixture: "n_plus_one_near_miss", serial: false, minCount: 10},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			out, manifest := topologyOf(t, tt.fixture)

			var expected struct {
				ParentSpanID string `json:"parent_span_id"`
				Operation    string `json:"operation"`
				Siblings     int    `json:"siblings"`
			}
			manifest.expect(t, &expected)

			group := siblings(out.Spans, expected.ParentSpanID, expected.Operation)
			require.Len(t, group, expected.Siblings,
				"the repeated group must survive the tool: the skill counts what it returns")
			assert.GreaterOrEqual(t, len(group), tt.minCount,
				"both fixtures must clear the count threshold, so only seriality separates them")

			sumUs, windowUs := serialityOf(t, group)
			if tt.serial {
				assert.LessOrEqual(t, sumUs, windowUs,
					"a real N+1 waits: durations must not exceed the window they occupy")
			} else {
				assert.Greater(t, sumUs, windowUs,
					"a fan-out overlaps: durations must exceed the window they occupy")
			}
			assert.Equal(t, manifest.PatternPresent, sumUs <= windowUs,
				"the seriality test alone must reproduce the fixture's label")
		})
	}
}

// TestSkillFixtures_ErrorLocus covers the two error fixtures. Both exist because
// the deepest-errored-span rule alone reaches the wrong answer on them, so the
// assertions pin the trap rather than the rule.
func TestSkillFixtures_ErrorLocus(t *testing.T) {
	t.Run("timeout_masked", func(t *testing.T) {
		out, manifest := errorsOf(t, "error_timeout_masked")

		var expected struct {
			LocusService   string `json:"locus_service"`
			LocusOperation string `json:"locus_operation"`
		}
		manifest.expect(t, &expected)

		require.Equal(t, 1, out.TotalErrorCount,
			"the trap is that only the proxy errored; more error spans would make the rule work")
		require.Len(t, out.Spans, 1)

		// get_trace_errors reaches the seeded failure...
		assert.Equal(t, "frontend-proxy", out.Spans[0].Service)

		// ...but the span that actually consumed the request is not in the error
		// list at all, which is why step 3 alone names the wrong service.
		assert.NotEqual(t, expected.LocusService, out.Spans[0].Service,
			"the locus must be absent from the error list, or the fixture is not adversarial")

		topology, _ := topologyOf(t, "error_timeout_masked")
		var locus *types.TopologySpan
		for i := range topology.Spans {
			if topology.Spans[i].Service == expected.LocusService &&
				topology.Spans[i].SpanName == expected.LocusOperation &&
				topology.Spans[i].DurationUs > 10_000_000 {
				locus = &topology.Spans[i]
				break
			}
		}
		require.NotNil(t, locus, "the real locus must be reachable from topology")
		assert.Equal(t, "Unset", locus.Status,
			"the locus carries no error status: that is what the timeout masks")
	})

	t.Run("sibling_errors", func(t *testing.T) {
		out, manifest := errorsOf(t, "sibling_errors")

		var expected struct {
			LocusService   string `json:"locus_service"`
			LocusOperation string `json:"locus_operation"`
		}
		manifest.expect(t, &expected)

		require.GreaterOrEqual(t, out.TotalErrorCount, 4,
			"both failing branches must be reported, or there is nothing to disambiguate")

		// Start-time order is the tie-breaker the span tree cannot supply.
		spans := slices.Clone(out.Spans)
		slices.SortFunc(spans, func(a, b types.SpanDetail) int {
			return strings.Compare(a.StartTime, b.StartTime)
		})

		var earliestLeaf *types.SpanDetail
		for i := range spans {
			if spans[i].Service == expected.LocusService {
				earliestLeaf = &spans[i]
				break
			}
		}
		require.NotNil(t, earliestLeaf)
		assert.Equal(t, expected.LocusOperation, earliestLeaf.SpanName)

		// The consequence failed later; ordering is what distinguishes them.
		assert.Less(t, spans[0].StartTime, spans[len(spans)-1].StartTime,
			"the fixture must span a start-time range for ordering to decide anything")
	})
}

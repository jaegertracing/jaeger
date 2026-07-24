// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// coreMetricFamilies is a golden list of metric family names that form a stable
// public monitoring surface for Jaeger v2 all-in-one / storage e2e binaries.
// These families are what monitoring/jaeger-mixin dashboards and production
// operators rely on; removing or renaming them is a backwards-incompatible change.
//
// Names match the Prometheus text exposition produced by the v2 binary (see
// scrapeMetrics). The Grafana mixin historically used slightly different
// suffixes (e.g. _total); the exposition names below are the source of truth.
//
// This is an incremental slice of https://github.com/jaegertracing/jaeger/issues/6278:
// full snapshot diffing already exists in CI; this check fails the e2e test
// immediately when a core family disappears.
var coreMetricFamilies = []string{
	"otelcol_receiver_accepted_spans",
	"otelcol_exporter_sent_spans",
	"jaeger_storage_requests",
	"jaeger_storage_latency",
}

// corePositiveCounters are counter families that must have at least one sample
// with value > 0 after a full span-store e2e run has exercised the pipeline.
// This catches regressions where a metric is registered but never incremented
// (see https://github.com/jaegertracing/jaeger/issues/7125).
var corePositiveCounters = []string{
	"otelcol_receiver_accepted_spans",
	"jaeger_storage_requests",
}

// metricExposition holds a parsed subset of a Prometheus text scrape.
type metricExposition struct {
	// families maps family name (from "# TYPE <name> ...") to its type string.
	families map[string]string
	// maxSampleValue maps sample metric name (may include _total/_count/_sum/_bucket
	// suffixes for some exporters) to the maximum observed sample value.
	maxSampleValue map[string]float64
}

func parseMetricExposition(body string) metricExposition {
	exp := metricExposition{
		families:       make(map[string]string),
		maxSampleValue: make(map[string]float64),
	}
	scanner := bufio.NewScanner(strings.NewReader(body))
	// Metrics lines can be long when many labels are present.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if strings.HasPrefix(line, "# TYPE ") {
			// # TYPE <name> <type>
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				exp.families[fields[2]] = fields[3]
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := parseSampleLine(line)
		if !ok {
			continue
		}
		if prev, exists := exp.maxSampleValue[name]; !exists || value > prev {
			exp.maxSampleValue[name] = value
		}
	}
	return exp
}

// parseSampleLine extracts the metric name and value from a Prometheus sample line.
// Supports both "name{labels} value" and "name value" forms; timestamps are ignored.
func parseSampleLine(line string) (name string, value float64, ok bool) {
	// Strip optional timestamp by taking the last whitespace-separated field as
	// value when two trailing numbers exist is ambiguous; Prometheus format is:
	//   metric_name[{labels}] value [timestamp]
	// Find the metric name first.
	rest := line
	if i := strings.IndexByte(line, '{'); i >= 0 {
		name = line[:i]
		end := strings.LastIndexByte(line, '}')
		if end < 0 {
			return "", 0, false
		}
		rest = strings.TrimSpace(line[end+1:])
	} else {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return "", 0, false
		}
		name = fields[0]
		rest = strings.Join(fields[1:], " ")
	}
	if name == "" || rest == "" {
		return "", 0, false
	}
	fields := strings.Fields(rest)
	if len(fields) < 1 {
		return "", 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", 0, false
	}
	return name, v, true
}

// sampleValueForFamily returns the max sample value for a family name, accepting
// common Prometheus suffixes that may appear on sample names (_total, _count).
func (e metricExposition) sampleValueForFamily(family string) (float64, bool) {
	candidates := []string{
		family,
		family + "_total",
		family + "_count",
	}
	var (
		max   float64
		found bool
	)
	for _, c := range candidates {
		if v, ok := e.maxSampleValue[c]; ok {
			if !found || v > max {
				max = v
				found = true
			}
		}
	}
	// Also consider histogram/summary bucket/sum samples under the family prefix.
	prefix := family + "_"
	for name, v := range e.maxSampleValue {
		if strings.HasPrefix(name, prefix) {
			if !found || v > max {
				max = v
				found = true
			}
		}
	}
	return max, found
}

// checkCoreMetrics returns an error when required metric families are missing
// from a Prometheus scrape, or when counters that should have been exercised are zero.
func checkCoreMetrics(body string) error {
	exp := parseMetricExposition(body)

	var missing []string
	for _, family := range coreMetricFamilies {
		if _, ok := exp.families[family]; !ok {
			missing = append(missing, family)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"core metrics missing from /metrics scrape (backwards-compat regression toward jaeger#6278): %v (present families: %d)",
			missing, len(exp.families))
	}

	var nonPositive []string
	for _, family := range corePositiveCounters {
		v, ok := exp.sampleValueForFamily(family)
		if !ok || v <= 0 {
			nonPositive = append(nonPositive, fmt.Sprintf("%s (max=%v present=%v)", family, v, ok))
		}
	}
	if len(nonPositive) > 0 {
		return fmt.Errorf("core metrics present but never incremented after e2e traffic: %v", nonPositive)
	}
	return nil
}

// assertCoreMetricsPresent fails the test when checkCoreMetrics reports a problem.
func assertCoreMetricsPresent(t *testing.T, body string) {
	t.Helper()
	require.NoError(t, checkCoreMetrics(body))
}

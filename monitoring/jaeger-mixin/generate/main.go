// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package main generates monitoring/jaeger-mixin/dashboard-for-grafana.json
// using the grafana-foundation-sdk Go builder API, producing native timeseries
// panels (React-based) to replace the deprecated Angular "graph" panels emitted
// by the old grafana-builder / Jsonnet toolchain.
//
// Usage:
//
//	cd monitoring/jaeger-mixin/generate
//	go run . > ../dashboard-for-grafana.json
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func main() {
	dash, err := buildDashboard()
	if err != nil {
		log.Fatalf("building dashboard: %v", err)
	}
	out, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		log.Fatalf("marshaling dashboard: %v", err)
	}

	// The SDK's AnnotationContainer.List carries omitempty, so it serialises to
	// "annotations": {} instead of "annotations": {"list": []}.
	// Unmarshal into a generic map, patch the annotations field to the correct
	// shape, then re-marshal so the output stays consistently indented.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		log.Fatalf("unmarshaling for annotation patch: %v", err)
	}
	annotationsWithList, err := json.Marshal(map[string]any{"list": []any{}})
	if err != nil {
		log.Fatalf("marshaling annotations patch: %v", err)
	}
	raw["annotations"] = annotationsWithList
	patched, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		log.Fatalf("re-marshaling patched dashboard: %v", err)
	}
	fmt.Println(string(patched))
}

func buildDashboard() (dashboard.Dashboard, error) {
	builder := dashboard.NewDashboardBuilder("Jaeger v2").
		Uid("jaeger-v2").
		Tags([]string{"jaeger"}).
		Editable().
		Refresh("10s").
		Time("now-1h", "now").
		Timezone(common.TimeZoneUtc).
		// Prometheus datasource selector — matches the original Jaeger dashboard
		// and other mixin dashboards in this repo.
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Data source").
				Type("prometheus"),
		).

		// ── Row 1: Collector - Ingestion ───────────────────────────────────────
		WithRow(dashboard.NewRowBuilder("Collector - Ingestion").Id(1)).
		WithPanel(spanIngestRatePanel()).
		WithPanel(spansRefusedPctPanel()).

		// ── Row 2: Collector - Export ──────────────────────────────────────────
		WithRow(dashboard.NewRowBuilder("Collector - Export").Id(4)).
		WithPanel(spanExportRatePanel()).
		WithPanel(exportSuccessRatePanel()).

		// ── Row 3: Storage ─────────────────────────────────────────────────────
		WithRow(dashboard.NewRowBuilder("Storage").Id(7)).
		WithPanel(storageRequestRatePanel()).
		WithPanel(storageLatencyP99Panel()).

		// ── Row 4: Query ───────────────────────────────────────────────────────
		WithRow(dashboard.NewRowBuilder("Query").Id(10)).
		WithPanel(queryRequestRatePanel()).
		WithPanel(queryLatencyP99Panel()).

		// ── Row 5: System ──────────────────────────────────────────────────────
		WithRow(dashboard.NewRowBuilder("System").Id(13)).
		WithPanel(cpuUsagePanel()).
		WithPanel(memoryRSSPanel())

	return builder.Build()
}

// datasourceRef returns a DataSourceRef that binds a panel to the dashboard's
// ${datasource} template variable, so the Grafana datasource selector affects
// all panels.
func datasourceRef() common.DataSourceRef {
	uid := "${datasource}"
	dsType := "prometheus"
	return common.DataSourceRef{Uid: &uid, Type: &dsType}
}

// stackedPanel returns a timeseries panel pre-configured for stacked mode.
// Use for rate/count panels where stacking multiple series is meaningful.
// All panels are bound to the ${datasource} variable and have Min(0) set.
func stackedPanel(id uint32, title string) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Span(12).
		Height(8).
		FillOpacity(10).
		Min(0).
		Datasource(datasourceRef()).
		Stacking(common.NewStackingConfigBuilder().
			Mode(common.StackingModeNormal))
}

// timeseriesPanel returns a timeseries panel without stacking.
// Use for latency percentiles and single-metric panels where stacking
// would produce meaningless visualizations.
// All panels are bound to the ${datasource} variable and have Min(0) set.
func timeseriesPanel(id uint32, title string) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Span(12).
		Height(8).
		FillOpacity(10).
		Min(0).
		Datasource(datasourceRef())
}

// promTarget is a shorthand for a Prometheus query with a legend.
func promTarget(expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().
		Expr(expr).
		LegendFormat(legend)
}

// ── Collector - Ingestion ──────────────────────────────────────────────────────

func spanIngestRatePanel() *timeseries.PanelBuilder {
	return stackedPanel(2, "Span Ingest Rate").
		Unit("ops").
		WithTarget(promTarget(
			`sum(rate(otelcol_receiver_refused_spans_total[1m])) or vector(0)`,
			"error",
		)).
		WithTarget(promTarget(
			`sum(rate(otelcol_receiver_accepted_spans_total[1m]))`,
			"success",
		))
}

func spansRefusedPctPanel() *timeseries.PanelBuilder {
	// Percentage metric — not stacked; stacking percentages can exceed 100%
	// and produces misleading visualizations.
	return timeseriesPanel(3, "% Spans Refused").
		Unit("percentunit").
		Max(1).
		WithTarget(promTarget(
			`sum(rate(otelcol_receiver_refused_spans_total[1m])) by (receiver, transport) `+
				`/ (sum(rate(otelcol_receiver_accepted_spans_total[1m])) by (receiver, transport) `+
				`+ sum(rate(otelcol_receiver_refused_spans_total[1m])) by (receiver, transport)) `+
				`or vector(0)`,
			"{{receiver}}-{{transport}}",
		))
}

// ── Collector - Export ────────────────────────────────────────────────────────

func spanExportRatePanel() *timeseries.PanelBuilder {
	return stackedPanel(5, "Span Export Rate").
		Unit("ops").
		WithTarget(promTarget(
			`sum(rate(otelcol_exporter_send_failed_spans_total[1m])) or vector(0)`,
			"error",
		)).
		WithTarget(promTarget(
			`sum(rate(otelcol_exporter_sent_spans_total[1m]))`,
			"success",
		))
}

func exportSuccessRatePanel() *timeseries.PanelBuilder {
	// Success-rate percentage — not stacked; stacking per-exporter percentages
	// can exceed 100% and makes the panel hard to interpret.
	return timeseriesPanel(6, "Export Success Rate %").
		Unit("percent").
		Max(100).
		WithTarget(promTarget(
			`(sum(rate(otelcol_exporter_sent_spans_total[1m])) by (exporter) `+
				`/ (sum(rate(otelcol_exporter_sent_spans_total[1m])) by (exporter) `+
				`+ sum(rate(otelcol_exporter_send_failed_spans_total[1m])) by (exporter))) `+
				`* 100 or vector(0)`,
			"{{exporter}}",
		))
}

// ── Storage ───────────────────────────────────────────────────────────────────

func storageRequestRatePanel() *timeseries.PanelBuilder {
	return stackedPanel(8, "Storage Request Rate").
		Unit("ops").
		WithTarget(promTarget(
			`sum(rate(jaeger_storage_requests_total[1m])) by (operation, result)`,
			"{{operation}} - {{result}}",
		))
}

func storageLatencyP99Panel() *timeseries.PanelBuilder {
	// Latency percentile — not stacked; stacking percentiles is misleading.
	return timeseriesPanel(9, "Storage Latency - P99").
		Unit("s").
		WithTarget(promTarget(
			`histogram_quantile(0.99, sum(rate(jaeger_storage_latency_seconds_bucket[1m])) by (le, operation))`,
			"{{operation}}",
		))
}

// ── Query ─────────────────────────────────────────────────────────────────────

func queryRequestRatePanel() *timeseries.PanelBuilder {
	return stackedPanel(11, "Query Request Rate").
		Unit("ops").
		WithTarget(promTarget(
			`sum(rate(http_server_request_duration_seconds_count{http_route="/api/traces"}[1m])) by (http_response_status_code)`,
			"status {{http_response_status_code}}",
		))
}

func queryLatencyP99Panel() *timeseries.PanelBuilder {
	// Latency percentile — not stacked; stacking percentiles is misleading.
	return timeseriesPanel(12, "Query Latency - P99").
		Unit("s").
		WithTarget(promTarget(
			`histogram_quantile(0.99, sum(rate(http_server_request_duration_seconds_bucket{http_route="/api/traces"}[1m])) by (le))`,
			"P99",
		))
}

// ── System ────────────────────────────────────────────────────────────────────

func cpuUsagePanel() *timeseries.PanelBuilder {
	// Single-metric panel — stacking a single series has no effect but is
	// semantically incorrect; use plain timeseries.
	return timeseriesPanel(14, "CPU Usage").
		Unit("percentunit").
		WithTarget(promTarget(
			`rate(otelcol_process_cpu_seconds_total[1m])`,
			"CPU",
		))
}

func memoryRSSPanel() *timeseries.PanelBuilder {
	// Single-metric panel — same rationale as cpuUsagePanel.
	return timeseriesPanel(15, "Memory RSS").
		Unit("bytes").
		WithTarget(promTarget(
			`otelcol_process_memory_rss_bytes`,
			"Memory",
		))
}

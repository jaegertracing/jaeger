// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package qualitymetrics

func GetSampleData() TQualityMetrics {
	return TQualityMetrics{
		TraceQualityDocumentationLink: "https://github.com/jaegertracing/jaeger-analytics-java#trace-quality-metrics",
		BannerText: &BannerText{
			Value: "🛑 Made up data, for demonstration only!",
			Styling: map[string]any{
				"color":      "blue",
				"fontWeight": "bold",
			},
		},
		Scores: []Score{
			{
				Key:   "coverage",
				Label: "Instrumentation Coverage",
				Max:   100,
				Value: 90,
			},
			{
				Key:   "quality",
				Label: "Trace Quality",
				Max:   100,
				Value: 85,
			},
		},
		Metrics: []Metric{
			{
				Name:                    "Span Completeness",
				Category:                "coverage",
				Description:             "Measures the completeness of spans in traces.",
				MetricDocumentationLink: "https://example.com/span-completeness-docs",
				MetricWeight:            0.6,
				PassCount:               1500,
				PassExamples: []Example{
					{TraceID: "trace-id-001"},
					{TraceID: "trace-id-002"},
				},
				FailureCount: 100,
				FailureExamples: []Example{
					{TraceID: "trace-id-003"},
				},
				Details: []Detail{
					{
						Description: "Detailed breakdown of span completeness.",
						Header:      "Span Completeness Details",
						Columns: []ColumnDef{
							{Key: "service", Label: "Service"},
							{Key: "spanCount", Label: "Span Count"},
						},
						Rows: []Row{
							{"service": "sample-service-A", "spanCount": 150},
							{"service": "sample-service-B", "spanCount": 200},
						},
					},
				},
			},
			{
				Name:                    "Error Rate",
				Category:                "quality",
				Description:             "Percentage of spans that resulted in errors.",
				MetricDocumentationLink: "https://example.com/error-rate-docs",
				MetricWeight:            0.4,
				PassCount:               1400,
				PassExamples: []Example{
					{TraceID: "trace-id-004"},
				},
				FailureCount: 80,
				FailureExamples: []Example{
					{TraceID: "trace-id-005"},
				},
				Details: []Detail{
					{
						Description: "Detailed breakdown of error rates.",
						Header:      "Error Rate Details",
						Columns: []ColumnDef{
							{Key: "service", Label: "Service"},
							{Key: "errorCount", Label: "Error Count"},
						},
						Rows: []Row{
							{"service": "sample-service-A", "errorCount": 5},
							{"service": "sample-service-B", "errorCount": 3},
						},
					},
				},
			},
		},
		Clients: []Client{
			{
				Version:    "1.0.0",
				MinVersion: "0.9.0",
				Count:      10,
				Examples: []Example{
					{TraceID: "trace-id-006"},
				},
			},
		},
	}
}

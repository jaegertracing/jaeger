// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

const (
	toolStatusOK              = "ok"
	toolStatusInvalidArgument = "invalid_argument"
	toolStatusNotFound        = "not_found"
	toolStatusError           = "error"
)

var allToolStatuses = []string{
	toolStatusOK,
	toolStatusInvalidArgument,
	toolStatusNotFound,
	toolStatusError,
}

type toolObservability struct {
	logger  *zap.Logger
	factory metrics.Factory

	mu    sync.Mutex
	tools map[string]*toolMetrics
}

type toolMetrics struct {
	inFlightGauge metrics.Gauge
	inFlightCount atomic.Int64
	statusMetrics map[string]*toolStatusMetrics
}

type toolStatusMetrics struct {
	Requests      metrics.Counter `metric:"requests"`
	Latency       metrics.Timer   `metric:"latency"`
	ResponseItems metrics.Counter `metric:"response_items"`
	ResponseBytes metrics.Counter `metric:"response_bytes"`
}

type requestSummary struct {
	traceID           string
	serviceName       string
	requestedLimit    int
	hasRequestedLimit bool
}

type responseSummary struct {
	resultCount       int
	hasResultCount    bool
	responseSizeBytes int
	hasResponseSize   bool
}

func newToolObservability(logger *zap.Logger, factory metrics.Factory) *toolObservability {
	if logger == nil {
		logger = zap.NewNop()
	}
	if factory == nil {
		factory = metrics.NullFactory
	}
	return &toolObservability{
		logger:  logger,
		factory: factory,
		tools:   make(map[string]*toolMetrics),
	}
}

func instrumentTool[In, Out any](
	obs *toolObservability,
	toolName string,
	handler mcp.ToolHandlerFor[In, Out],
) mcp.ToolHandlerFor[In, Out] {
	if obs == nil {
		return handler
	}

	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		metricsForTool := obs.metricsForTool(toolName)
		reqSummary := summarizeRequest(input)
		start := time.Now()

		metricsForTool.inFlightGauge.Update(metricsForTool.inFlightCount.Add(1))
		startFields := append([]zap.Field{zap.String("tool_name", toolName)}, reqSummary.logFields()...)
		obs.logger.Debug("MCP tool invocation started", startFields...)

		defer func() {
			metricsForTool.inFlightGauge.Update(metricsForTool.inFlightCount.Add(-1))

			duration := time.Since(start)
			status := normalizeToolStatus(err, result)
			statusMetrics := metricsForTool.status(status)
			statusMetrics.Requests.Inc(1)
			statusMetrics.Latency.Record(duration)

			respSummary := summarizeResponse(output, status)
			if respSummary.hasResultCount {
				statusMetrics.ResponseItems.Inc(int64(respSummary.resultCount))
			}
			if respSummary.hasResponseSize {
				statusMetrics.ResponseBytes.Inc(int64(respSummary.responseSizeBytes))
			}

			doneFields := []zap.Field{
				zap.String("tool_name", toolName),
				zap.String("status", status),
				zap.Duration("duration", duration),
			}
			doneFields = append(doneFields, reqSummary.logFields()...)
			if respSummary.hasResultCount {
				doneFields = append(doneFields, zap.Int("result_count", respSummary.resultCount))
			}
			if respSummary.hasResponseSize {
				doneFields = append(doneFields, zap.Int("response_size_bytes", respSummary.responseSizeBytes))
			}

			if err != nil {
				obs.logger.Error("MCP tool invocation failed", append(doneFields, zap.Error(err))...)
				return
			}
			if result != nil && result.IsError {
				resultErr := result.GetError()
				if resultErr != nil {
					doneFields = append(doneFields, zap.Error(resultErr))
				}
				obs.logger.Error("MCP tool invocation failed", doneFields...)
				return
			}
			obs.logger.Debug("MCP tool invocation completed", doneFields...)
		}()

		return handler(ctx, req, input)
	}
}

func (o *toolObservability) metricsForTool(toolName string) *toolMetrics {
	o.mu.Lock()
	defer o.mu.Unlock()

	if existing, ok := o.tools[toolName]; ok {
		return existing
	}

	toolScope := o.factory.Namespace(metrics.NSOptions{
		Name: "",
		Tags: map[string]string{"tool_name": toolName},
	})
	m := &toolMetrics{
		inFlightGauge: toolScope.Gauge(metrics.Options{Name: "in_flight_requests"}),
		statusMetrics: make(map[string]*toolStatusMetrics, len(allToolStatuses)),
	}
	for _, status := range allToolStatuses {
		sm := &toolStatusMetrics{}
		scoped := toolScope.Namespace(metrics.NSOptions{
			Name: "",
			Tags: map[string]string{"status": status},
		})
		metrics.MustInit(sm, scoped, nil)
		m.statusMetrics[status] = sm
	}

	o.tools[toolName] = m
	return m
}

func (m *toolMetrics) status(status string) *toolStatusMetrics {
	if metricsForStatus, ok := m.statusMetrics[status]; ok {
		return metricsForStatus
	}
	return m.statusMetrics[toolStatusError]
}

func normalizeToolStatus(err error, result *mcp.CallToolResult) string {
	if err == nil && (result == nil || !result.IsError) {
		return toolStatusOK
	}

	message := ""
	switch {
	case err != nil:
		message = strings.ToLower(err.Error())
	case result != nil && result.GetError() != nil:
		message = strings.ToLower(result.GetError().Error())
	default:
		// Keep empty message when the SDK did not provide a concrete error.
	}

	if strings.Contains(message, "not found") {
		return toolStatusNotFound
	}
	if strings.Contains(message, "invalid") ||
		strings.Contains(message, "required") ||
		strings.Contains(message, "must") ||
		strings.Contains(message, "malformed") ||
		strings.Contains(message, "unsupported") ||
		strings.Contains(message, "empty") {
		return toolStatusInvalidArgument
	}
	return toolStatusError
}

func summarizeRequest(input any) requestSummary {
	v := reflectValue(input)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return requestSummary{}
	}

	summary := requestSummary{
		traceID:     fieldString(v, "TraceID"),
		serviceName: fieldString(v, "ServiceName"),
	}

	for _, limitField := range []string{"Limit", "SearchDepth"} {
		if limit, ok := fieldPositiveInt(v, limitField); ok {
			summary.requestedLimit = limit
			summary.hasRequestedLimit = true
			return summary
		}
	}
	if size, ok := fieldNonEmptySliceLen(v, "SpanIDs"); ok {
		summary.requestedLimit = size
		summary.hasRequestedLimit = true
	}
	return summary
}

func summarizeResponse(output any, status string) responseSummary {
	if status != toolStatusOK {
		return responseSummary{}
	}

	summary := responseSummary{}
	if count, ok := inferResultCount(output); ok {
		summary.resultCount = count
		summary.hasResultCount = true
	}
	if size, ok := payloadSize(output); ok {
		summary.responseSizeBytes = size
		summary.hasResponseSize = true
	}
	return summary
}

func inferResultCount(output any) (int, bool) {
	v := reflectValue(output)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return 0, false
	}

	for _, field := range []string{"Services", "SpanNames", "Traces", "Segments", "Spans"} {
		if count, ok := fieldSliceLen(v, field); ok {
			return count, true
		}
	}

	rootSpan, rootExists := fieldValue(v, "RootSpan")
	orphans, orphansExists := fieldValue(v, "Orphans")
	if rootExists || orphansExists {
		return countTreeNodes(rootSpan) + countTreeNodes(orphans), true
	}

	return 0, false
}

func countTreeNodes(value reflect.Value) int {
	value = unwrapValue(value)
	if !value.IsValid() {
		return 0
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		total := 0
		for i := 0; i < value.Len(); i++ {
			total += countTreeNodes(value.Index(i))
		}
		return total
	case reflect.Struct:
		total := 1
		children, ok := fieldValue(value, "Children")
		if ok {
			total += countTreeNodes(children)
		}
		return total
	default:
		return 0
	}
}

func payloadSize(output any) (int, bool) {
	bytes, err := json.Marshal(output)
	if err != nil {
		return 0, false
	}
	return len(bytes), true
}

func (s requestSummary) logFields() []zap.Field {
	fields := make([]zap.Field, 0, 3)
	if s.traceID != "" {
		fields = append(fields, zap.String("trace_id", s.traceID))
	}
	if s.serviceName != "" {
		fields = append(fields, zap.String("service_name", s.serviceName))
	}
	if s.hasRequestedLimit {
		fields = append(fields, zap.Int("requested_limit", s.requestedLimit))
	}
	return fields
}

func reflectValue(value any) reflect.Value {
	return unwrapValue(reflect.ValueOf(value))
}

func unwrapValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func fieldValue(v reflect.Value, name string) (reflect.Value, bool) {
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return reflect.Value{}, false
	}
	return f, true
}

func fieldString(v reflect.Value, name string) string {
	f, ok := fieldValue(v, name)
	if !ok || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

func fieldPositiveInt(v reflect.Value, name string) (int, bool) {
	f, ok := fieldValue(v, name)
	if !ok || f.Kind() != reflect.Int {
		return 0, false
	}
	if f.Int() <= 0 {
		return 0, false
	}
	return int(f.Int()), true
}

func fieldSliceLen(v reflect.Value, name string) (int, bool) {
	f, ok := fieldValue(v, name)
	if !ok || (f.Kind() != reflect.Slice && f.Kind() != reflect.Array) {
		return 0, false
	}
	return f.Len(), true
}

func fieldNonEmptySliceLen(v reflect.Value, name string) (int, bool) {
	size, ok := fieldSliceLen(v, name)
	if !ok || size == 0 {
		return 0, false
	}
	return size, true
}

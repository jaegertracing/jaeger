// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
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

var errToolHandlerPanic = errors.New("tool handler panicked")

type toolObservability struct {
	logger  *zap.Logger
	factory metrics.Factory

	mu    sync.Mutex
	tools map[string]*toolMetrics
}

type toolMetrics struct {
	statusMetrics map[string]*toolStatusMetrics
}

type toolStatusMetrics struct {
	ResponseItems metrics.Counter `metric:"response_items"`
}

type requestSummary struct {
	traceID           string
	serviceName       string
	requestedLimit    int
	hasRequestedLimit bool
}

type responseSummary struct {
	resultCount    int
	hasResultCount bool
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
	metricsForTool := obs.metricsForTool(toolName)

	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		start := time.Now()
		var reqSummary requestSummary
		reqSummarySet := false
		getReqSummary := func() requestSummary {
			if !reqSummarySet {
				reqSummary = summarizeRequest(input)
				reqSummarySet = true
			}
			return reqSummary
		}

		defer func() {
			panicValue := recover()
			if panicValue != nil {
				err = errToolHandlerPanic
			}

			duration := time.Since(start)
			status := normalizeToolStatus(err, result)
			addOTelToolLabels(ctx, toolName, status)

			respSummary := summarizeResponse(output, status)
			if respSummary.hasResultCount {
				metricsForTool.status(status).ResponseItems.Inc(int64(respSummary.resultCount))
			}

			buildDoneFields := func() []zap.Field {
				fields := []zap.Field{
					zap.String("tool_name", toolName),
					zap.String("status", status),
					zap.Duration("duration", duration),
				}
				fields = append(fields, getReqSummary().logFields()...)
				if respSummary.hasResultCount {
					fields = append(fields, zap.Int("result_count", respSummary.resultCount))
				}
				return fields
			}

			if panicValue != nil {
				obs.logger.Error("MCP tool invocation failed", append(buildDoneFields(), zap.Any("panic", panicValue), zap.Error(err))...)
				return
			}

			if err != nil {
				obs.logFailure(status, append(buildDoneFields(), zap.Error(err))...)
				return
			}
			if result != nil && result.IsError {
				fields := buildDoneFields()
				if resultErr := result.GetError(); resultErr != nil {
					fields = append(fields, zap.Error(resultErr))
				}
				obs.logFailure(status, fields...)
				return
			}

			if ce := obs.logger.Check(zap.DebugLevel, "MCP tool invocation completed"); ce != nil {
				ce.Write(buildDoneFields()...)
			}
		}()

		return handler(ctx, req, input)
	}
}

func (o *toolObservability) logFailure(status string, fields ...zap.Field) {
	if status == toolStatusInvalidArgument || status == toolStatusNotFound {
		o.logger.Warn("MCP tool invocation failed", fields...)
		return
	}
	o.logger.Error("MCP tool invocation failed", fields...)
}

func addOTelToolLabels(ctx context.Context, toolName, status string) {
	labeler, ok := otelhttp.LabelerFromContext(ctx)
	if !ok {
		return
	}
	labeler.Add(
		attribute.String("mcp.tool_name", toolName),
		attribute.String("mcp.status", status),
	)
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
	m := &toolMetrics{statusMetrics: make(map[string]*toolStatusMetrics, len(allToolStatuses))}
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
	switch v := input.(type) {
	case types.GetServicesInput:
		return summarizeRequestWithFields("", "", v.Limit)
	case *types.GetServicesInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields("", "", v.Limit)
	case types.GetSpanNamesInput:
		return summarizeRequestWithFields("", v.ServiceName, v.Limit)
	case *types.GetSpanNamesInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields("", v.ServiceName, v.Limit)
	case types.SearchTracesInput:
		return summarizeRequestWithFields("", v.ServiceName, v.SearchDepth)
	case *types.SearchTracesInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields("", v.ServiceName, v.SearchDepth)
	case types.GetTraceTopologyInput:
		return summarizeRequestWithFields(v.TraceID, "", v.Depth)
	case *types.GetTraceTopologyInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields(v.TraceID, "", v.Depth)
	case types.GetSpanDetailsInput:
		return summarizeRequestWithFields(v.TraceID, "", len(v.SpanIDs))
	case *types.GetSpanDetailsInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields(v.TraceID, "", len(v.SpanIDs))
	case types.GetTraceErrorsInput:
		return summarizeRequestWithFields(v.TraceID, "", 0)
	case *types.GetTraceErrorsInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields(v.TraceID, "", 0)
	case types.GetCriticalPathInput:
		return summarizeRequestWithFields(v.TraceID, "", 0)
	case *types.GetCriticalPathInput:
		if v == nil {
			return requestSummary{}
		}
		return summarizeRequestWithFields(v.TraceID, "", 0)
	default:
		return requestSummary{}
	}
}

func summarizeRequestWithFields(traceID, serviceName string, requestedLimit int) requestSummary {
	summary := requestSummary{
		traceID:     traceID,
		serviceName: serviceName,
	}
	if requestedLimit > 0 {
		summary.requestedLimit = requestedLimit
		summary.hasRequestedLimit = true
	}
	return summary
}

func summarizeResponse(output any, status string) responseSummary {
	if status != toolStatusOK {
		return responseSummary{}
	}

	resultCount, ok := inferResultCount(output)
	if !ok {
		return responseSummary{}
	}
	return responseSummary{
		resultCount:    resultCount,
		hasResultCount: true,
	}
}

func inferResultCount(output any) (int, bool) {
	switch v := output.(type) {
	case types.GetServicesOutput:
		return len(v.Services), true
	case *types.GetServicesOutput:
		if v == nil {
			return 0, false
		}
		return len(v.Services), true
	case types.GetSpanNamesOutput:
		return len(v.SpanNames), true
	case *types.GetSpanNamesOutput:
		if v == nil {
			return 0, false
		}
		return len(v.SpanNames), true
	case types.SearchTracesOutput:
		return len(v.Traces), true
	case *types.SearchTracesOutput:
		if v == nil {
			return 0, false
		}
		return len(v.Traces), true
	case types.GetCriticalPathOutput:
		return len(v.Segments), true
	case *types.GetCriticalPathOutput:
		if v == nil {
			return 0, false
		}
		return len(v.Segments), true
	case types.GetSpanDetailsOutput:
		return len(v.Spans), true
	case *types.GetSpanDetailsOutput:
		if v == nil {
			return 0, false
		}
		return len(v.Spans), true
	case types.GetTraceErrorsOutput:
		return len(v.Spans), true
	case *types.GetTraceErrorsOutput:
		if v == nil {
			return 0, false
		}
		return len(v.Spans), true
	case types.GetTraceTopologyOutput:
		return countTopologyNodes(v), true
	case *types.GetTraceTopologyOutput:
		if v == nil {
			return 0, false
		}
		return countTopologyNodes(*v), true
	default:
		return 0, false
	}
}

func countTopologyNodes(output types.GetTraceTopologyOutput) int {
	return countSpanNode(output.RootSpan) + countSpanNodes(output.Orphans)
}

func countSpanNodes(value any) int {
	switch nodes := value.(type) {
	case []*types.SpanNode:
		total := 0
		for _, n := range nodes {
			total += countSpanNode(n)
		}
		return total
	case []types.SpanNode:
		total := 0
		for i := range nodes {
			total += countSpanNode(&nodes[i])
		}
		return total
	default:
		return 0
	}
}

func countSpanNode(value any) int {
	switch node := value.(type) {
	case *types.SpanNode:
		if node == nil {
			return 0
		}
		total := 1
		for _, child := range node.Children {
			total += countSpanNode(child)
		}
		return total
	case types.SpanNode:
		n := node
		return countSpanNode(&n)
	default:
		return 0
	}
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

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts registers MCP prompts that encode common investigation
// workflows. MCP prompts are user-controlled conversation starters (unlike
// tools which are model-controlled). Clients surface them as slash commands
// or menu items, letting users select a workflow and fill in parameters
// before the LLM begins reasoning.
func (s *server) registerPrompts() {
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "investigate_service",
		Description: "Step-by-step workflow for investigating a service using progressive disclosure.",
		Arguments: []*mcp.PromptArgument{
			{Name: "service_name", Description: "Service name to investigate", Required: true},
		},
	}, investigateServicePrompt)

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "investigate_errors",
		Description: "Diagnose error patterns for a service by finding error traces and drilling into root causes.",
		Arguments: []*mcp.PromptArgument{
			{Name: "service_name", Description: "Service name to investigate", Required: true},
		},
	}, investigateErrorsPrompt)

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "investigate_latency",
		Description: "Identify latency bottlenecks for a service using critical path analysis.",
		Arguments: []*mcp.PromptArgument{
			{Name: "service_name", Description: "Service name to investigate", Required: true},
			{Name: "duration_min", Description: "Minimum trace duration to filter for (e.g. 500ms, 2s)", Required: false},
		},
	}, investigateLatencyPrompt)
}

func investigateServicePrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	service := req.Params.Arguments["service_name"]
	if service == "" {
		return nil, errors.New("service_name is required")
	}

	return &mcp.GetPromptResult{
		Description: "Investigate service: " + service,
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(
						`Investigate the service "%s" in Jaeger. Follow this workflow:

1. Call search_traces with service_name="%s" and start_time_min="-1h" to find recent traces.
2. Pick the trace with the highest span count or longest duration.
3. Call get_trace_topology with that trace_id to see the structural overview.
4. Identify which spans look suspicious (errors, high duration).
5. Call get_span_details for those specific spans to see full attributes.
6. Summarize what this service is doing, which downstream services it calls, and any issues found.`,
						service, service),
				},
			},
		},
	}, nil
}

func investigateErrorsPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	service := req.Params.Arguments["service_name"]
	if service == "" {
		return nil, errors.New("service_name is required")
	}

	return &mcp.GetPromptResult{
		Description: "Investigate errors for service: " + service,
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(
						`Diagnose error patterns for the service "%s" in Jaeger. Follow this workflow:

1. Call search_traces with service_name="%s", with_errors=true, and start_time_min="-1h".
2. Pick a trace that has errors.
3. Call get_trace_errors with that trace_id to see all error spans.
4. Identify whether errors originate in "%s" or propagate from a downstream service.
5. Call get_span_details for the earliest error span to see the root cause attributes and events.
6. Summarize the error pattern: where it originates, how it propagates, and what the error message says.`,
						service, service, service),
				},
			},
		},
	}, nil
}

func investigateLatencyPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	service := req.Params.Arguments["service_name"]
	if service == "" {
		return nil, errors.New("service_name is required")
	}

	durationMin := req.Params.Arguments["duration_min"]
	durationFilter := ""
	if durationMin != "" {
		durationFilter = fmt.Sprintf(", duration_min=%q", durationMin)
	}

	return &mcp.GetPromptResult{
		Description: "Investigate latency for service: " + service,
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(
						`Identify latency bottlenecks for the service "%s" in Jaeger. Follow this workflow:

1. Call search_traces with service_name="%s", start_time_min="-1h"%s to find slow traces.
2. Pick the slowest trace.
3. Call get_trace_topology with that trace_id to see the service call graph.
4. Call get_critical_path with that trace_id to find the blocking execution path.
5. The critical path shows which spans contributed most to total latency (highest self_time_us).
6. Call get_span_details for the top self_time_us spans on the critical path.
7. Summarize where time is spent and which service/operation is the bottleneck.`,
						service, service, durationFilter),
				},
			},
		},
	}, nil
}

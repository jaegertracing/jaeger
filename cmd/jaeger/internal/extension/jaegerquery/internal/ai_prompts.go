// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import "fmt"

const TraceQueryJSONSchema = `
{
	"type": "object",
	"properties": {
		"service": {
			"type": "string",
			"description": "The name of the service (e.g., 'frontend', 'payment-service')."
		},
		"operation": {
			"type": "string",
			"description": "The name of the operation or endpoint (e.g., '/api/v1/charge')."
		},
		"tags": {
			"type": "object",
			"additionalProperties": { "type": "string" },
			"description": "Key-value pairs of trace attributes (e.g., {'http.status_code': '500', 'error': 'true'})."
		},
		"minDuration": {
			"type": "string",
			"description": "Minimum duration of the trace (e.g., '2s', '500ms')."
		},
		"maxDuration": {
			"type": "string",
			"description": "Maximum duration of the trace (e.g., '5s')."
		}
	},
	"required": []
}
`

// buildSearchAnalysisPrompt constructs the LLM prompt to translate natural language into Jaeger Search JSON parameters.
func buildSearchAnalysisPrompt(question string) string {
	return fmt.Sprintf(`You are an expert Jaeger tracing query assistant.
Your ONLY job is to translate the user's natural language request into a strictly formatted JSON object matching the Jaeger search parameters schema.
Do not output ANY conversational text. Do not explain your reasoning. Output ONLY valid JSON.

Schema:
%s

User Request: "%s"

Analyze the request and extract the parameters into JSON. Use standard open telemetry semantic conventions for tags if applicable.`, TraceQueryJSONSchema, question)
}

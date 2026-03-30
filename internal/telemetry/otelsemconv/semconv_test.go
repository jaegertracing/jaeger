// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelsemconv

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestServiceNameAttribute(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected attribute.KeyValue
	}{
		{
			name:     "valid service name",
			value:    "my-service",
			expected: semconv.ServiceNameKey.String("my-service"),
		},
		{
			name:     "empty service name",
			value:    "",
			expected: semconv.ServiceNameKey.String(""),
		},
		{
			name:     "service name with spaces",
			value:    "my service name",
			expected: semconv.ServiceNameKey.String("my service name"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ServiceNameAttribute(tt.value)
			if result.Key != tt.expected.Key {
				t.Errorf("Expected key %v, got %v", tt.expected.Key, result.Key)
			}
			if result.Value != tt.expected.Value {
				t.Errorf("Expected value %v, got %v", tt.expected.Value, result.Value)
			}
		})
	}
}

func TestPeerServiceAttribute(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected attribute.KeyValue
	}{
		{
			name:     "valid peer service",
			value:    "external-api",
			expected: semconv.ServicePeerNameKey.String("external-api"),
		},
		{
			name:     "empty peer service",
			value:    "",
			expected: semconv.ServicePeerNameKey.String(""),
		},
		{
			name:     "peer service with special characters",
			value:    "api-service_v1.2",
			expected: semconv.ServicePeerNameKey.String("api-service_v1.2"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PeerServiceAttribute(tt.value)
			if result.Key != tt.expected.Key {
				t.Errorf("Expected key %v, got %v", tt.expected.Key, result.Key)
			}
			if result.Value != tt.expected.Value {
				t.Errorf("Expected value %v, got %v", tt.expected.Value, result.Value)
			}
		})
	}
}

func TestDBSystemAttribute(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected attribute.KeyValue
	}{
		{
			name:     "postgresql database",
			value:    "postgresql",
			expected: semconv.DBSystemNameKey.String("postgresql"),
		},
		{
			name:     "mysql database",
			value:    "mysql",
			expected: semconv.DBSystemNameKey.String("mysql"),
		},
		{
			name:     "empty database system",
			value:    "",
			expected: semconv.DBSystemNameKey.String(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DBSystemAttribute(tt.value)
			if result.Key != tt.expected.Key {
				t.Errorf("Expected key %v, got %v", tt.expected.Key, result.Key)
			}
			if result.Value != tt.expected.Value {
				t.Errorf("Expected value %v, got %v", tt.expected.Value, result.Value)
			}
		})
	}
}

func TestHTTPStatusCodeAttribute(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		expected attribute.KeyValue
	}{
		{
			name:     "success status code",
			value:    200,
			expected: semconv.HTTPResponseStatusCodeKey.Int(200),
		},
		{
			name:     "client error status code",
			value:    404,
			expected: semconv.HTTPResponseStatusCodeKey.Int(404),
		},
		{
			name:     "server error status code",
			value:    500,
			expected: semconv.HTTPResponseStatusCodeKey.Int(500),
		},
		{
			name:     "zero status code",
			value:    0,
			expected: semconv.HTTPResponseStatusCodeKey.Int(0),
		},
		{
			name:     "negative status code",
			value:    -1,
			expected: semconv.HTTPResponseStatusCodeKey.Int(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTTPStatusCodeAttribute(tt.value)
			if result.Key != tt.expected.Key {
				t.Errorf("Expected key %v, got %v", tt.expected.Key, result.Key)
			}
			if result.Value != tt.expected.Value {
				t.Errorf("Expected value %v, got %v", tt.expected.Value, result.Value)
			}
		})
	}
}

func TestAttributeTypes(t *testing.T) {
	// Test that all helper functions return the correct attribute types
	serviceAttr := ServiceNameAttribute("test")
	if serviceAttr.Value.Type() != attribute.STRING {
		t.Errorf("ServiceNameAttribute should return STRING type, got %v", serviceAttr.Value.Type())
	}

	peerAttr := PeerServiceAttribute("test")
	if peerAttr.Value.Type() != attribute.STRING {
		t.Errorf("PeerServiceAttribute should return STRING type, got %v", peerAttr.Value.Type())
	}

	dbAttr := DBSystemAttribute("test")
	if dbAttr.Value.Type() != attribute.STRING {
		t.Errorf("DBSystemAttribute should return STRING type, got %v", dbAttr.Value.Type())
	}

	httpAttr := HTTPStatusCodeAttribute(200)
	if httpAttr.Value.Type() != attribute.INT64 {
		t.Errorf("HTTPStatusCodeAttribute should return INT64 type, got %v", httpAttr.Value.Type())
	}
}

func TestMCPGenAIAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attr     attribute.KeyValue
		wantKey  string
		wantType attribute.Type
		wantVal  string
	}{
		{
			name:     "mcp method",
			attr:     McpMethodName("tools/call"),
			wantKey:  McpMethodNameKey,
			wantType: attribute.STRING,
			wantVal:  "tools/call",
		},
		{
			name:     "mcp session id",
			attr:     McpSessionID("session-123"),
			wantKey:  McpSessionIDKey,
			wantType: attribute.STRING,
			wantVal:  "session-123",
		},
		{
			name:     "gen ai tool",
			attr:     GenAIToolName("search_traces"),
			wantKey:  GenAIToolNameKey,
			wantType: attribute.STRING,
			wantVal:  "search_traces",
		},
		{
			name:     "error type",
			attr:     ErrorType("tool_error"),
			wantKey:  ErrorTypeKey,
			wantType: attribute.STRING,
			wantVal:  "tool_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.attr.Key) != tt.wantKey {
				t.Fatalf("expected key %s, got %s", tt.wantKey, tt.attr.Key)
			}
			if tt.attr.Value.Type() != tt.wantType {
				t.Fatalf("expected type %v, got %v", tt.wantType, tt.attr.Value.Type())
			}
			if tt.attr.Value.AsString() != tt.wantVal {
				t.Fatalf("expected value %q, got %q", tt.wantVal, tt.attr.Value.AsString())
			}
		})
	}
}

func TestGenAIOperationNameExecuteTool(t *testing.T) {
	if string(GenAIOperationNameExecuteTool.Key) != GenAIOperationNameKey {
		t.Fatalf("expected key %s, got %s", GenAIOperationNameKey, GenAIOperationNameExecuteTool.Key)
	}
	if GenAIOperationNameExecuteTool.Value.AsString() != "execute_tool" {
		t.Fatalf("expected value execute_tool, got %s", GenAIOperationNameExecuteTool.Value.AsString())
	}
}

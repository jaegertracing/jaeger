// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// getRoot is a test helper to extract the RootSpan from GetTraceTopologyOutput
func getRoot(t *testing.T, output types.GetTraceTopologyOutput) *types.SpanNode {
	t.Helper()
	if output.RootSpan == nil {
		return nil
	}
	root, ok := output.RootSpan.(*types.SpanNode)
	require.True(t, ok, "RootSpan should be a *SpanNode")
	return root
}

// getOrphans is a test helper to extract orphans from GetTraceTopologyOutput
func getOrphans(t *testing.T, output types.GetTraceTopologyOutput) []*types.SpanNode {
	t.Helper()
	if output.Orphans == nil {
		return nil
	}
	orphans, ok := output.Orphans.([]*types.SpanNode)
	require.True(t, ok, "Orphans should be []*SpanNode")
	return orphans
}

func TestGetTraceTopologyHandler_Handle_Success(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	child1SpanID := "child01"
	child2SpanID := "child02"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/checkout",
			attributes: map[string]string{
				"http.method": "GET",
			},
		},
		{
			spanID:       child1SpanID,
			parentSpanID: rootSpanID,
			operation:    "getCart",
		},
		{
			spanID:       child2SpanID,
			parentSpanID: rootSpanID,
			operation:    "processPayment",
			hasError:     true,
			errorMessage: "Payment failed",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0, // Full tree
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	root := getRoot(t, output)

	// Verify root span
	assert.Equal(t, "/api/checkout", root.SpanName)
	assert.Equal(t, "Ok", root.Status)
	assert.Len(t, root.Children, 2)

	// Verify children are present (order not guaranteed)
	operations := make(map[string]*types.SpanNode)
	for _, child := range root.Children {
		operations[child.SpanName] = child
	}

	assert.Contains(t, operations, "getCart")
	assert.Contains(t, operations, "processPayment")
	assert.Equal(t, "Ok", operations["getCart"].Status)
	assert.Equal(t, "Error", operations["processPayment"].Status)
}

func TestGetTraceTopologyHandler_Handle_DepthLimit(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	child1SpanID := "child01"
	grandchildSpanID := "gchild1"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/checkout",
		},
		{
			spanID:       child1SpanID,
			parentSpanID: rootSpanID,
			operation:    "getCart",
		},
		{
			spanID:       grandchildSpanID,
			parentSpanID: child1SpanID,
			operation:    "queryDB",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	tests := []struct {
		name                 string
		depth                int
		expectRoot           bool
		expectChild          bool
		expectGchild         bool
		expectRootTruncated  int
		expectChildTruncated int
	}{
		{
			name:                 "depth 0 returns full tree",
			depth:                0,
			expectRoot:           true,
			expectChild:          true,
			expectGchild:         true,
			expectRootTruncated:  0,
			expectChildTruncated: 0,
		},
		{
			name:                 "depth 1 returns only root",
			depth:                1,
			expectRoot:           true,
			expectChild:          false,
			expectGchild:         false,
			expectRootTruncated:  1, // 1 child truncated at root
			expectChildTruncated: 0,
		},
		{
			name:                 "depth 2 returns root and children",
			depth:                2,
			expectRoot:           true,
			expectChild:          true,
			expectGchild:         false,
			expectRootTruncated:  0,
			expectChildTruncated: 1, // 1 grandchild truncated at child level
		},
		{
			name:                 "depth 3 returns full tree",
			depth:                3,
			expectRoot:           true,
			expectChild:          true,
			expectGchild:         true,
			expectRootTruncated:  0,
			expectChildTruncated: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := types.GetTraceTopologyInput{
				TraceID: traceID,
				Depth:   tt.depth,
			}

			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

			require.NoError(t, err)
			root := getRoot(t, output)

			// Check root
			assert.Equal(t, "/api/checkout", root.SpanName)

			// Check truncated children count at root level
			assert.Equal(t, tt.expectRootTruncated, root.TruncatedChildren)

			// Check children
			if tt.expectChild {
				assert.Len(t, root.Children, 1)
				assert.Equal(t, "getCart", root.Children[0].SpanName)

				// Check truncated children count at child level
				assert.Equal(t, tt.expectChildTruncated, root.Children[0].TruncatedChildren)

				// Check grandchildren
				if tt.expectGchild {
					assert.Len(t, root.Children[0].Children, 1)
					assert.Equal(t, "queryDB", root.Children[0].Children[0].SpanName)
				} else {
					assert.Empty(t, root.Children[0].Children)
				}
			} else {
				assert.Empty(t, root.Children)
			}
		})
	}
}

func TestGetTraceTopologyHandler_Handle_MultipleChildren(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	// Create a trace with one root and multiple children
	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "root",
		},
		{
			spanID:       "child01",
			parentSpanID: rootSpanID,
			operation:    "child1",
		},
		{
			spanID:       "child02",
			parentSpanID: rootSpanID,
			operation:    "child2",
		},
		{
			spanID:       "child03",
			parentSpanID: rootSpanID,
			operation:    "child3",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	root := getRoot(t, output)
	assert.Equal(t, "root", root.SpanName)
	assert.Len(t, root.Children, 3)

	// Verify all children are present
	operations := make(map[string]bool)
	for _, child := range root.Children {
		operations[child.SpanName] = true
	}
	assert.True(t, operations["child1"])
	assert.True(t, operations["child2"])
	assert.True(t, operations["child3"])
}

func TestGetTraceTopologyHandler_Handle_ComplexTree(t *testing.T) {
	traceID := testTraceID

	// Create a more complex tree structure:
	//     root
	//    /    \
	//   A      B
	//  / \      \
	// C   D      E
	spanConfigs := []spanConfig{
		{spanID: "root001", operation: "root"},
		{spanID: "spanAAA", parentSpanID: "root001", operation: "A"},
		{spanID: "spanBBB", parentSpanID: "root001", operation: "B"},
		{spanID: "spanCCC", parentSpanID: "spanAAA", operation: "C"},
		{spanID: "spanDDD", parentSpanID: "spanAAA", operation: "D"},
		{spanID: "spanEEE", parentSpanID: "spanBBB", operation: "E"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	root := getRoot(t, output)

	// Verify structure
	assert.Equal(t, "root", root.SpanName)
	assert.Len(t, root.Children, 2)

	// Find A and B
	var nodeA, nodeB *types.SpanNode
	for _, child := range root.Children {
		switch child.SpanName {
		case "A":
			nodeA = child
		case "B":
			nodeB = child
		default:
			// ignore other operations
		}
	}

	require.NotNil(t, nodeA)
	require.NotNil(t, nodeB)

	// Verify A's children (C and D)
	assert.Len(t, nodeA.Children, 2)
	operations := make(map[string]bool)
	for _, child := range nodeA.Children {
		operations[child.SpanName] = true
	}
	assert.True(t, operations["C"])
	assert.True(t, operations["D"])

	// Verify B's child (E)
	assert.Len(t, nodeB.Children, 1)
	assert.Equal(t, "E", nodeB.Children[0].SpanName)
}

func TestGetTraceTopologyHandler_Handle_SingleSpan(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/simple",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	root := getRoot(t, output)
	assert.Equal(t, "/api/simple", root.SpanName)
	assert.Empty(t, root.Children)
}

func TestGetTraceTopologyHandler_Handle_NoAttributes(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	// Create a span with attributes
	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/test",
			attributes: map[string]string{
				"http.method":      "GET",
				"http.status_code": "200",
				"user.id":          "12345",
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	root := getRoot(t, output)

	// Verify that the SpanNode doesn't have an Attributes field
	// This is ensured by the type definition, but we verify the structure is correct
	assert.Equal(t, "/api/test", root.SpanName)
	assert.Equal(t, "Ok", root.Status)
}

func TestGetTraceTopologyHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetTraceTopologyHandler(nil)

	input := types.GetTraceTopologyInput{}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetTraceTopologyHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetTraceTopologyHandler(nil)

	input := types.GetTraceTopologyInput{
		TraceID: "invalid-trace-id",
	}

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetTraceTopologyHandler_Handle_TraceNotFound(t *testing.T) {
	mock := newMockYieldingEmpty()

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: testTraceID,
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetTraceTopologyHandler_Handle_QueryError(t *testing.T) {
	mock := newMockYieldingError(errors.New("database connection failed"))

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: testTraceID,
	}

	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetTraceTopologyHandler_Handle_MultipleIterations(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	childSpanID := "child01"

	// Create traces with different spans that will be merged
	testTrace1 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: rootSpanID, operation: "/api/root"},
	})
	testTrace2 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: childSpanID, parentSpanID: rootSpanID, operation: "/api/child"},
	})

	mock := &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				// Yield multiple batches successfully - they should be merged
				yield([]ptrace.Traces{testTrace1}, nil)
				yield([]ptrace.Traces{testTrace2}, nil)
			}
		},
	}

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	// Should succeed and build the complete tree
	require.NoError(t, err)
	root := getRoot(t, output)
	assert.Equal(t, "/api/root", root.SpanName)
	assert.Len(t, root.Children, 1)
	assert.Equal(t, "/api/child", root.Children[0].SpanName)
}

func TestGetTraceTopologyHandler_Handle_NoRootSpan(t *testing.T) {
	traceID := testTraceID

	// Create a trace where all spans have parents (no root)
	// This is an invalid trace, but we should handle it gracefully by returning orphans
	spanConfigs := []spanConfig{
		{spanID: "child01", parentSpanID: "nonexistent", operation: "orphan1"},
		{spanID: "child02", parentSpanID: "nonexistent", operation: "orphan2"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	rootSpan := getRoot(t, output)
	assert.Nil(t, rootSpan, "Should have no root span")

	orphans := getOrphans(t, output)
	require.NotNil(t, orphans)
	assert.Len(t, orphans, 2, "Should have 2 orphans")

	// Verify orphans are present
	operations := make(map[string]bool)
	for _, orphan := range orphans {
		operations[orphan.SpanName] = true
	}
	assert.True(t, operations["orphan1"])
	assert.True(t, operations["orphan2"])
}

func TestGetTraceTopologyHandler_Handle_WithOrphans(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	childSpanID := "child01"
	orphanSpanID := "orphan01"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/root",
		},
		{
			spanID:       childSpanID,
			parentSpanID: rootSpanID,
			operation:    "child",
		},
		{
			spanID:       orphanSpanID,
			parentSpanID: "nonexistent",
			operation:    "orphan",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	rootSpan := getRoot(t, output)
	require.NotNil(t, rootSpan)
	assert.Equal(t, "/api/root", rootSpan.SpanName)
	assert.Len(t, rootSpan.Children, 1)
	assert.Equal(t, "child", rootSpan.Children[0].SpanName)

	// Verify orphan is present
	orphans := getOrphans(t, output)
	require.NotNil(t, orphans)
	assert.Len(t, orphans, 1)
	assert.Equal(t, "orphan", orphans[0].SpanName)
}

func TestGetTraceTopologyHandler_Handle_ErrorStatus(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	errorSpanID := "error01"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/checkout",
		},
		{
			spanID:       errorSpanID,
			parentSpanID: rootSpanID,
			operation:    "processPayment",
			hasError:     true,
			errorMessage: "Payment gateway timeout",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	root := getRoot(t, output)

	// Find the error span
	assert.Len(t, root.Children, 1)
	errorNode := root.Children[0]
	assert.Equal(t, "processPayment", errorNode.SpanName)
	assert.Equal(t, "Error", errorNode.Status)
}

func TestGetTraceTopologyHandler_Handle_PreservesTimingInfo(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	spanConfigs := []spanConfig{
		{
			spanID:    rootSpanID,
			operation: "/api/test",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := newMockYieldingTraces(testTrace)

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{
		TraceID: traceID,
		Depth:   0,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	root := getRoot(t, output)

	// Verify timing fields are present
	assert.NotEmpty(t, root.StartTime)
	assert.NotZero(t, root.DurationUs)
}

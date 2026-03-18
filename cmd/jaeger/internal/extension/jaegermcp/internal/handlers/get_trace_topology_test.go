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

// findSpanByName is a test helper that looks up a TopologySpan by its SpanName field.
func findSpanByName(spans []types.TopologySpan, spanName string) *types.TopologySpan {
	for i := range spans {
		if spans[i].SpanName == spanName {
			return &spans[i]
		}
	}
	return nil
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

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	require.Len(t, output.Spans, 3)

	root := findSpanByName(output.Spans, "/api/checkout")
	require.NotNil(t, root)
	assert.Equal(t, "Ok", root.Status)

	getCart := findSpanByName(output.Spans, "getCart")
	require.NotNil(t, getCart)
	assert.Equal(t, "Ok", getCart.Status)
	assert.Equal(t, root.Path+"/"+spanIDToHex(child1SpanID), getCart.Path)

	payment := findSpanByName(output.Spans, "processPayment")
	require.NotNil(t, payment)
	assert.Equal(t, "Error", payment.Status)
	assert.Equal(t, root.Path+"/"+spanIDToHex(child2SpanID), payment.Path)
}

func TestGetTraceTopologyHandler_Handle_DepthLimit(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	child1SpanID := "child01"
	grandchildSpanID := "gchild1"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "/api/checkout"},
		{spanID: child1SpanID, parentSpanID: rootSpanID, operation: "getCart"},
		{spanID: grandchildSpanID, parentSpanID: child1SpanID, operation: "queryDB"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	tests := []struct {
		name                 string
		depth                int
		expectSpanNames      []string
		dontExpectNames      []string
		expectRootTruncated  int
		expectChildTruncated int
	}{
		{
			name:                 "depth 0 returns full tree",
			depth:                0,
			expectSpanNames:      []string{"/api/checkout", "getCart", "queryDB"},
			expectRootTruncated:  0,
			expectChildTruncated: 0,
		},
		{
			name:                 "depth 1 returns only root",
			depth:                1,
			expectSpanNames:      []string{"/api/checkout"},
			dontExpectNames:      []string{"getCart", "queryDB"},
			expectRootTruncated:  1, // 1 child truncated at root
			expectChildTruncated: 0,
		},
		{
			name:                 "depth 2 returns root and children",
			depth:                2,
			expectSpanNames:      []string{"/api/checkout", "getCart"},
			dontExpectNames:      []string{"queryDB"},
			expectRootTruncated:  0,
			expectChildTruncated: 1, // 1 grandchild truncated at child level
		},
		{
			name:                 "depth 3 returns full tree",
			depth:                3,
			expectSpanNames:      []string{"/api/checkout", "getCart", "queryDB"},
			expectRootTruncated:  0,
			expectChildTruncated: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := types.GetTraceTopologyInput{TraceID: traceID, Depth: tt.depth}
			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
			require.NoError(t, err)

			byName := make(map[string]types.TopologySpan)
			for _, s := range output.Spans {
				byName[s.SpanName] = s
			}
			for _, name := range tt.expectSpanNames {
				assert.Contains(t, byName, name, "expected span %q in output", name)
			}
			for _, name := range tt.dontExpectNames {
				assert.NotContains(t, byName, name, "did not expect span %q in output", name)
			}

			// Verify TruncatedChildren counts
			if root, ok := byName["/api/checkout"]; ok {
				assert.Equal(t, tt.expectRootTruncated, root.TruncatedChildren)
			}
			if child, ok := byName["getCart"]; ok {
				assert.Equal(t, tt.expectChildTruncated, child.TruncatedChildren)
			}
		})
	}
}

func TestGetTraceTopologyHandler_Handle_MultipleChildren(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "root"},
		{spanID: "child01", parentSpanID: rootSpanID, operation: "child1"},
		{spanID: "child02", parentSpanID: rootSpanID, operation: "child2"},
		{spanID: "child03", parentSpanID: rootSpanID, operation: "child3"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 4)

	names := make(map[string]bool)
	for _, s := range output.Spans {
		names[s.SpanName] = true
	}
	assert.True(t, names["child1"])
	assert.True(t, names["child2"])
	assert.True(t, names["child3"])
}

func TestGetTraceTopologyHandler_Handle_ComplexTree(t *testing.T) {
	traceID := testTraceID

	// Tree structure:
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

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 6)

	// Verify paths encode parent-child relationships.
	root := findSpanByName(output.Spans, "root")
	require.NotNil(t, root)

	nodeA := findSpanByName(output.Spans, "A")
	require.NotNil(t, nodeA)
	assert.Equal(t, root.Path+"/"+spanIDToHex("spanAAA"), nodeA.Path)

	nodeC := findSpanByName(output.Spans, "C")
	require.NotNil(t, nodeC)
	assert.Equal(t, nodeA.Path+"/"+spanIDToHex("spanCCC"), nodeC.Path)

	nodeE := findSpanByName(output.Spans, "E")
	require.NotNil(t, nodeE)
	nodeB := findSpanByName(output.Spans, "B")
	require.NotNil(t, nodeB)
	assert.Equal(t, nodeB.Path+"/"+spanIDToHex("spanEEE"), nodeE.Path)
}

func TestGetTraceTopologyHandler_Handle_PathEncoding(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	childSpanID := "child01"
	grandchildSpanID := "gchild1"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "root"},
		{spanID: childSpanID, parentSpanID: rootSpanID, operation: "child"},
		{spanID: grandchildSpanID, parentSpanID: childSpanID, operation: "grandchild"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 3)

	rootHex := spanIDToHex(rootSpanID)
	childHex := spanIDToHex(childSpanID)
	grandchildHex := spanIDToHex(grandchildSpanID)

	// The root span's path is just its own hex-encoded span ID.
	root := findSpanByName(output.Spans, "root")
	require.NotNil(t, root)
	assert.Equal(t, rootHex, root.Path)

	// A child appends its hex ID.
	child := findSpanByName(output.Spans, "child")
	require.NotNil(t, child)
	assert.Equal(t, rootHex+"/"+childHex, child.Path)

	// A grandchild extends further.
	grandchild := findSpanByName(output.Spans, "grandchild")
	require.NotNil(t, grandchild)
	assert.Equal(t, rootHex+"/"+childHex+"/"+grandchildHex, grandchild.Path)
}

func TestGetTraceTopologyHandler_Handle_SingleSpan(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "/api/simple"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	require.Len(t, output.Spans, 1)
	assert.Equal(t, "/api/simple", output.Spans[0].SpanName)
	assert.Equal(t, spanIDToHex(rootSpanID), output.Spans[0].Path)
}

func TestGetTraceTopologyHandler_Handle_NoAttributes(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

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

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	// Verify that attributes are NOT included in TopologySpan (enforced by the type).
	assert.Equal(t, "/api/test", output.Spans[0].SpanName)
	assert.Equal(t, "Ok", output.Spans[0].Status)
}

func TestGetTraceTopologyHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetTraceTopologyHandler(nil)

	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, types.GetTraceTopologyInput{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetTraceTopologyHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetTraceTopologyHandler(nil)

	input := types.GetTraceTopologyInput{TraceID: "invalid-trace-id"}
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetTraceTopologyHandler_Handle_TraceNotFound(t *testing.T) {
	mock := newMockYieldingEmpty()
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: testTraceID}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetTraceTopologyHandler_Handle_QueryError(t *testing.T) {
	mock := newMockYieldingError(errors.New("database connection failed"))
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: testTraceID}
	_, _, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetTraceTopologyHandler_Handle_MultipleIterations(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	childSpanID := "child01"

	testTrace1 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: rootSpanID, operation: "/api/root"},
	})
	testTrace2 := createTestTraceWithSpans(traceID, []spanConfig{
		{spanID: childSpanID, parentSpanID: rootSpanID, operation: "/api/child"},
	})

	mock := &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace1}, nil)
				yield([]ptrace.Traces{testTrace2}, nil)
			}
		},
	}

	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 2)

	root := findSpanByName(output.Spans, "/api/root")
	require.NotNil(t, root)

	child := findSpanByName(output.Spans, "/api/child")
	require.NotNil(t, child)
	assert.Equal(t, spanIDToHex(rootSpanID)+"/"+spanIDToHex(childSpanID), child.Path)
}

func TestGetTraceTopologyHandler_Handle_NoRootSpan(t *testing.T) {
	traceID := testTraceID

	// All spans reference a parent that is not in the trace (orphan scenario).
	spanConfigs := []spanConfig{
		{spanID: "child01", parentSpanID: "nonexistent", operation: "orphan1"},
		{spanID: "child02", parentSpanID: "nonexistent", operation: "orphan2"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 2)

	// Orphan paths should include the missing parent hex ID as a prefix.
	missingParentHex := spanIDToHex("nonexistent")
	for _, s := range output.Spans {
		assert.Contains(t, s.Path, missingParentHex+"/", "orphan path should start with missing parent ID")
	}
}

func TestGetTraceTopologyHandler_Handle_WithOrphans(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	childSpanID := "child01"
	orphanSpanID := "orphan01"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "/api/root"},
		{spanID: childSpanID, parentSpanID: rootSpanID, operation: "child"},
		{spanID: orphanSpanID, parentSpanID: "nonexistent", operation: "orphan"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 3)

	root := findSpanByName(output.Spans, "/api/root")
	require.NotNil(t, root)
	assert.Equal(t, spanIDToHex(rootSpanID), root.Path)

	child := findSpanByName(output.Spans, "child")
	require.NotNil(t, child)
	assert.Equal(t, spanIDToHex(rootSpanID)+"/"+spanIDToHex(childSpanID), child.Path)

	orphan := findSpanByName(output.Spans, "orphan")
	require.NotNil(t, orphan)
	assert.Equal(t, spanIDToHex("nonexistent")+"/"+spanIDToHex(orphanSpanID), orphan.Path)
}

func TestGetTraceTopologyHandler_Handle_ErrorStatus(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"
	errorSpanID := "error01"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "/api/checkout"},
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

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 2)

	errorSpan := findSpanByName(output.Spans, "processPayment")
	require.NotNil(t, errorSpan)
	assert.Equal(t, "Error", errorSpan.Status)
}

func TestGetTraceTopologyHandler_Handle_PreservesTimingInfo(t *testing.T) {
	traceID := testTraceID
	rootSpanID := "root001"

	spanConfigs := []spanConfig{
		{spanID: rootSpanID, operation: "/api/test"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)
	mock := newMockYieldingTraces(testTrace)
	handler := &getTraceTopologyHandler{queryService: mock}

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 1)
	assert.NotEmpty(t, output.Spans[0].StartTime)
	assert.NotZero(t, output.Spans[0].DurationUs)
}

func TestGetTraceTopologyHandler_Handle_DFSOrder(t *testing.T) {
	traceID := testTraceID

	// Tree:  root -> A -> C
	//                 \-> D
	//             \-> B -> E
	// DFS order: root, A, C, D, B, E
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

	input := types.GetTraceTopologyInput{TraceID: traceID, Depth: 0}
	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Spans, 6)

	// The root must come before its children; each parent before its children.
	indexOf := make(map[string]int)
	for i, s := range output.Spans {
		indexOf[s.SpanName] = i
	}
	assert.Less(t, indexOf["root"], indexOf["A"])
	assert.Less(t, indexOf["root"], indexOf["B"])
	assert.Less(t, indexOf["A"], indexOf["C"])
	assert.Less(t, indexOf["A"], indexOf["D"])
	assert.Less(t, indexOf["B"], indexOf["E"])
	// DFS: A and its subtree come before B.
	assert.Less(t, indexOf["C"], indexOf["B"])
	assert.Less(t, indexOf["D"], indexOf["B"])
}

// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/otelsemconv"
)

const (
	warningDuplicateSpanID     = "duplicate span IDs; skipping clock skew adjustment"
	warningMissingParentSpanID = "parent span ID=%s is not in the trace; skipping clock skew adjustment"
	warningMaxDeltaExceeded    = "max clock skew adjustment delta of %v exceeded; not applying calculated delta of %v"
	warningSkewAdjustDisabled  = "clock skew adjustment disabled; not applying calculated delta of %v"
)

// CorrectClockSkew returns an Adjuster that corrects span timestamps for clock skew.
//
// This adjuster modifies the start and log timestamps of child spans that are
// inconsistent with their parent spans due to clock differences between hosts.
// It assumes all spans have unique IDs and should be used after SpanIDUniquifier.
//
// The adjuster determines if two spans belong to the same source by deriving a
// unique string representation of a host based on resource attributes,
// such as `host.id`, `host.ip`, or `host.name`.
// If two spans have the same host key, they are considered to be from
// the same source, and no clock skew adjustment is expected between them.
//
// Parameters:
//   - maxDelta: The maximum allowable time adjustment. Adjustments exceeding
//     this value will be ignored.
func CorrectClockSkew(maxDelta time.Duration) Adjuster {
	return Func(func(traces ptrace.Traces) {
		adjuster := &clockSkewAdjuster{
			traces:   traces,
			maxDelta: maxDelta,
		}
		adjuster.buildNodesMap()
		adjuster.buildSubGraphs()
		for _, root := range adjuster.roots {
			skew := clockSkew{hostKey: root.hostKey}
			adjuster.adjustNode(root, nil, skew)
		}
	})
}

type clockSkewAdjuster struct {
	traces   ptrace.Traces
	maxDelta time.Duration
	spans    map[pcommon.SpanID]*node
	roots    map[pcommon.SpanID]*node
}

type clockSkew struct {
	delta   time.Duration
	hostKey string
}

type node struct {
	span     ptrace.Span
	children []*node
	hostKey  string
}

// hostKey derives a unique string representation of a host based on resource attributes.
// This is used to determine if two spans are from the same host.
func hostKey(resource ptrace.ResourceSpans) string {
	if attr, ok := resource.Resource().Attributes().Get(string(otelsemconv.HostIDKey)); ok {
		return attr.Str()
	}
	if attr, ok := resource.Resource().Attributes().Get(string(otelsemconv.HostIPKey)); ok {
		if attr.Type() == pcommon.ValueTypeStr {
			return attr.Str()
		} else if attr.Type() == pcommon.ValueTypeSlice {
			ips := attr.Slice()
			if ips.Len() > 0 {
				return ips.At(0).AsString()
			}
		}
	}
	if attr, ok := resource.Resource().Attributes().Get(string(otelsemconv.HostNameKey)); ok {
		return attr.Str()
	}
	return ""
}

// buildNodesMap creates a mapping of span IDs to their corresponding nodes.
func (a *clockSkewAdjuster) buildNodesMap() {
	a.spans = make(map[pcommon.SpanID]*node)
	resources := a.traces.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		resource := resources.At(i)
		hk := hostKey(resource)
		scopes := resource.ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			spans := scopes.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if _, exists := a.spans[span.SpanID()]; exists {
					jptrace.AddWarnings(span, warningDuplicateSpanID)
				} else {
					a.spans[span.SpanID()] = &node{
						span:    span,
						hostKey: hk,
					}
				}
			}
		}
	}
}

// finds all spans that have no parent, i.e. where parentID is either 0
// or points to an ID for which there is no span.
func (a *clockSkewAdjuster) buildSubGraphs() {
	a.roots = make(map[pcommon.SpanID]*node)
	for _, n := range a.spans {
		if n.span.ParentSpanID() == pcommon.NewSpanIDEmpty() {
			a.roots[n.span.SpanID()] = n
			continue
		}
		if p, ok := a.spans[n.span.ParentSpanID()]; ok {
			p.children = append(p.children, n)
		} else {
			warning := fmt.Sprintf(warningMissingParentSpanID, n.span.ParentSpanID())
			jptrace.AddWarnings(n.span, warning)
			// treat spans with invalid parent ID as root spans
			a.roots[n.span.SpanID()] = n
		}
	}
}

func (a *clockSkewAdjuster) adjustNode(n *node, parent *node, skew clockSkew) {
	if (n.hostKey != skew.hostKey || n.hostKey == "") && parent != nil {
		// Node n is from a different host. The parent has already been adjusted,
		// so we can compare this node's timestamps against the parent.
		skew = clockSkew{
			hostKey: n.hostKey,
			delta:   a.calculateSkew(n, parent),
		}
	}
	a.adjustTimestamps(n, skew)
	for _, child := range n.children {
		a.adjustNode(child, n, skew)
	}
}

func (*clockSkewAdjuster) calculateSkew(child *node, parent *node) time.Duration {
	parentStartTime := parent.span.StartTimestamp().AsTime()
	childStartTime := child.span.StartTimestamp().AsTime()
	parentEndTime := parent.span.EndTimestamp().AsTime()
	childEndTime := child.span.EndTimestamp().AsTime()
	parentDuration := parentEndTime.Sub(parentStartTime)
	childDuration := childEndTime.Sub(childStartTime)

	if childDuration > parentDuration {
		// When the child lasted longer than the parent, it was either
		// async or the parent may have timed out before child responded.
		// The only reasonable adjustment we can do in this case is to make
		// sure the child does not start before parent.
		if childStartTime.Before(parentStartTime) {
			return parentStartTime.Sub(childStartTime)
		}
		return 0
	}
	if !childStartTime.Before(parentStartTime) && !childEndTime.After(parentEndTime) {
		// child already fits within the parent span, do not adjust
		return 0
	}
	// Assume that network latency is equally split between req and res.
	latency := (parentDuration - childDuration) / 2
	// Goal: parentStartTime + latency = childStartTime + adjustment
	return parentStartTime.Add(latency).Sub(childStartTime)
}

func (a *clockSkewAdjuster) adjustTimestamps(n *node, skew clockSkew) {
	if skew.delta == 0 {
		return
	}
	if absDuration(skew.delta) > a.maxDelta {
		if a.maxDelta == 0 {
			jptrace.AddWarnings(n.span, fmt.Sprintf(warningSkewAdjustDisabled, skew.delta))
			return
		}
		jptrace.AddWarnings(n.span, fmt.Sprintf(warningMaxDeltaExceeded, a.maxDelta, skew.delta))
		return
	}
	n.span.SetStartTimestamp(pcommon.NewTimestampFromTime(n.span.StartTimestamp().AsTime().Add(skew.delta)))
	jptrace.AddWarnings(n.span, fmt.Sprintf("This span's timestamps were adjusted by %v", skew.delta))
	for i := 0; i < n.span.Events().Len(); i++ {
		event := n.span.Events().At(i)
		event.SetTimestamp(pcommon.NewTimestampFromTime(event.Timestamp().AsTime().Add(skew.delta)))
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -1 * d
	}
	return d
}

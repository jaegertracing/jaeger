// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	warningDuplicateSpanID       = "duplicate span IDs; skipping clock skew adjustment"
	warningFormatInvalidParentID = "invalid parent span IDs=%s; skipping clock skew adjustment"
	warningMaxDeltaExceeded      = "max clock skew adjustment delta of %v exceeded; not applying calculated delta of %v"
	warningSkewAdjustDisabled    = "clock skew adjustment disabled; not applying calculated delta of %v"
)

// ClockSkew returns an adjuster that modifies start time and log timestamps
// for spans that appear to be "off" with respect to the parent span due to
// clock skew on different servers. The main condition that it checks is that
// child spans do not start before or end after their parent spans.
//
// The algorithm assumes that all spans have unique IDs, so this adjuster should be used after
// SpanIDUniquifier.
func ClockSkew(maxDelta time.Duration) Adjuster {
	return Func(func(traces ptrace.Traces) {
		adjuster := &clockSkewAdjuster{
			traces:   traces,
			maxDelta: maxDelta,
		}
		adjuster.buildNodesMap()
		adjuster.buildSubGraphs()
		for _, n := range adjuster.roots {
			skew := clockSkew{hostKey: n.hostKey}
			adjuster.adjustNode(n, nil, skew)
		}
	})
}

type clockSkewAdjuster struct {
	traces   ptrace.Traces
	spans    map[pcommon.SpanID]*node
	roots    map[pcommon.SpanID]*node
	maxDelta time.Duration
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

// hostKey returns a string representation of the host identity that can be used
// to determine if two spans originated from the same host.
//
// TODO convert process tags to a canonical format somewhere else
func hostKey(resource ptrace.ResourceSpans) string {
	if attr, ok := resource.Resource().Attributes().Get("ip"); ok {
		if attr.Type() == pcommon.ValueTypeStr {
			return attr.Str()
		}
		if attr.Type() == pcommon.ValueTypeInt {
			var buf [4]byte // avoid heap allocation
			ip := buf[0:4]  // utils require a slice, not an array
			//nolint: gosec // G115
			binary.BigEndian.PutUint32(ip, uint32(attr.Int()))
			return net.IP(ip).String()
		}
		if attr.Type() == pcommon.ValueTypeBytes {
			if l := attr.Bytes().Len(); l == 4 || l == 16 {
				return net.IP(attr.Bytes().AsRaw()).String()
			}
		}
	}
	return ""
}

// buildNodesMap builds a map of span IDs -> node{}.
func (a *clockSkewAdjuster) buildNodesMap() {
	a.spans = make(map[pcommon.SpanID]*node)
	resourceSpans := a.traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resources := resourceSpans.At(i)
		hk := hostKey(resources)
		scopes := resources.ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			scope := scopes.At(j)
			spans := scope.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if _, ok := a.spans[span.SpanID()]; ok {
					jptrace.AddWarning(span, warningDuplicateSpanID)
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
			warning := fmt.Sprintf(warningFormatInvalidParentID, n.span.ParentSpanID())
			jptrace.AddWarning(n.span, warning)
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
			jptrace.AddWarning(n.span, fmt.Sprintf(warningSkewAdjustDisabled, skew.delta))
			return
		}
		jptrace.AddWarning(n.span, fmt.Sprintf(warningMaxDeltaExceeded, a.maxDelta, skew.delta))
		return
	}
	n.span.SetStartTimestamp(pcommon.NewTimestampFromTime(n.span.StartTimestamp().AsTime().Add(skew.delta)))
	jptrace.AddWarning(n.span, fmt.Sprintf("This span's timestamps were adjusted by %v", skew.delta))
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

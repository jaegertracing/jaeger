// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adjuster

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/uber/jaeger/model"
)

// ClockSkew returns an adjuster that modifies start time and log timestamps
// for spans that appear to be "off" with respect to the parent span due to
// clock skew on different servers. The main condition that it checks is that
// child spans do not start before or end after their parent spans.
//
// The algorithm assumes that all spans have unique IDs, so the trace may need
// to go through another adjuster first, such as SpanIDDeduper.
//
// This adjuster never returns any errors. Instead it records any issues
// it encounters in Span.Warnings.
func ClockSkew() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		adjuster := &clockSkewAdjuster{
			trace: trace,
		}
		adjuster.buildNodesMap()
		adjuster.buildSubGraphs()
		for _, n := range adjuster.roots {
			skew := clockSkew{hostKey: n.hostKey}
			adjuster.adjustNode(n, nil, skew)
		}
		return adjuster.trace, nil
	})
}

const (
	warningDuplicateSpanID       = "duplicate span IDs; skipping clock skew adjustment"
	warningFormatInvalidParentID = "invalid parent span IDs=%s; skipping clock skew adjustment"
)

type clockSkewAdjuster struct {
	trace *model.Trace
	spans map[model.SpanID]*node
	roots map[model.SpanID]*node
}

type clockSkew struct {
	delta   time.Duration
	hostKey string
}

type node struct {
	span     *model.Span
	children []*node
	hostKey  string
}

// hostKey returns a string representation of the host identity that can be used
// to determine if two spans originated from the same host.
//
// TODO convert process tags to a canonical format somewhere else
func hostKey(span *model.Span) string {
	if tag, ok := span.Process.Tags.FindByKey("ip"); ok {
		if tag.VType == model.StringType {
			return tag.VStr
		}
		if tag.VType == model.Int64Type {
			var buf [4]byte // avoid heap allocation
			ip := buf[0:4]  // utils require a slice, not an array
			binary.BigEndian.PutUint32(ip, uint32(tag.Int64()))
			return net.IP(ip).String()
		}
		if tag.VType == model.BinaryType {
			if l := len(tag.Binary()); l == 4 || l == 16 {
				return net.IP(tag.Binary()).String()
			}
		}
	}
	return ""
}

// buildNodesMap builds a map of span IDs -> node{}.
func (a *clockSkewAdjuster) buildNodesMap() {
	a.spans = make(map[model.SpanID]*node)
	for _, span := range a.trace.Spans {
		if _, ok := a.spans[span.SpanID]; ok {
			span.Warnings = append(span.Warnings, warningDuplicateSpanID)
		} else {
			a.spans[span.SpanID] = &node{
				span:    span,
				hostKey: hostKey(span),
			}
		}
	}
}

// finds all spans that have no parent, i.e. where parentID is either 0
// or points to an ID for which there is no span.
func (a *clockSkewAdjuster) buildSubGraphs() {
	a.roots = make(map[model.SpanID]*node)
	for _, n := range a.spans {
		// TODO handle FOLLOWS_FROM references
		if n.span.ParentSpanID == 0 {
			a.roots[n.span.SpanID] = n
			continue
		}
		if p, ok := a.spans[n.span.ParentSpanID]; ok {
			p.children = append(p.children, n)
		} else {
			warning := fmt.Sprintf(warningFormatInvalidParentID, n.span.ParentSpanID)
			n.span.Warnings = append(n.span.Warnings, warning)
			// Treat spans with invalid parent ID as root spans
			a.roots[n.span.SpanID] = n
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

func (a *clockSkewAdjuster) calculateSkew(child *node, parent *node) time.Duration {
	parentDuration := parent.span.Duration
	childDuration := child.span.Duration
	parentEndTime := parent.span.StartTime.Add(parent.span.Duration)
	childEndTime := child.span.StartTime.Add(child.span.Duration)

	if childDuration > parentDuration {
		// When the child lasted longer than the parent, it was either
		// async or the parent may have timed out before child responded.
		// The only reasonable adjustment we can do in this case is to make
		// sure the child does not start before parent.
		if child.span.StartTime.Before(parent.span.StartTime) {
			return parent.span.StartTime.Sub(child.span.StartTime)
		}
		return 0
	}
	if !child.span.StartTime.Before(parent.span.StartTime) && !childEndTime.After(parentEndTime) {
		// child already fits within the parent span, do not adjust
		return 0
	}
	// Assume that network latency is equally split between req and res.
	latency := (parentDuration - childDuration) / 2
	// Goal: parentStartTime + latency = childStartTime + adjustment
	return parent.span.StartTime.Add(latency).Sub(child.span.StartTime)
}

func (a *clockSkewAdjuster) adjustTimestamps(n *node, skew clockSkew) {
	n.span.StartTime = n.span.StartTime.Add(skew.delta)
	for i := range n.span.Logs {
		n.span.Logs[i].Timestamp = n.span.Logs[i].Timestamp.Add(skew.delta)
	}
}

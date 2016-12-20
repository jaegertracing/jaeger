// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package adjuster

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/multierror"
)

// ClockSkew returns an adjuster that modifies start time and log timestamps
// for spans that appear to be "off" with respect to the parent span due to
// clock skew on different servers. The main condition that it checks is that
// child spans do not start before or end after their parent spans.
//
// The algorithm assumes that all spans have unique IDs, otherwise it returns
// an error, so the trace may need to go through another adjuster first.
func ClockSkew() Adjuster {
	return Func(adjustClockSkew)
}

func adjustClockSkew(trace *model.Trace) (*model.Trace, error) {
	adjuster := &clockSkewAdjuster{
		trace: trace,
	}
	adjuster.mapIDsToSpans()
	adjuster.buildSubGraphs()
	for _, n := range adjuster.roots {
		skew := clockSkew{hostKey: n.hostKey}
		adjuster.adjustNode(n, nil, skew)
	}
	return adjuster.trace, multierror.Wrap(adjuster.errors)
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

type clockSkewAdjuster struct {
	trace  *model.Trace
	spans  map[model.SpanID]*node
	roots  map[model.SpanID]*node
	errors []error
}

type clockSkew struct {
	delta   int64
	hostKey string
}

// mapIDsToSpans builds a map of span IDs -> node{}.
func (a *clockSkewAdjuster) mapIDsToSpans() {
	a.spans = make(map[model.SpanID]*node)
	for _, span := range a.trace.Spans {
		if _, ok := a.spans[span.SpanID]; ok {
			a.errors = append(a.errors, fmt.Errorf("found more than one span with ID=%x", span.SpanID))
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
			err := fmt.Errorf("invalid parent span ID=%x in the span with ID=%x", n.span.ParentSpanID, n.span.SpanID)
			a.errors = append(a.errors, err)
			// Treat spans with invalid parent ID as root spans
			a.roots[n.span.SpanID] = n
		}
	}
}

func (a *clockSkewAdjuster) adjustNode(n *node, parent *node, skew clockSkew) {
	if (n.hostKey != skew.hostKey || n.hostKey == "") && parent != nil {
		// Node n is from a differnt host. The parent has already been adjusted,
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

func (a *clockSkewAdjuster) calculateSkew(child *node, parent *node) int64 {
	parentDuration := parent.span.Duration
	childDuration := child.span.Duration
	parentEndTime := parent.span.StartTime + parent.span.Duration
	childEndTime := child.span.StartTime + child.span.Duration

	if childDuration > parentDuration {
		// When the child lasted longer than the parent, it was either
		// async or the parent may have timed out before child responded.
		// The only reasonable adjustment we can do in this case is to make
		// sure the child does not start before parent.
		if child.span.StartTime < parent.span.StartTime {
			return int64(parent.span.StartTime - child.span.StartTime)
		}
		return 0
	}
	if child.span.StartTime >= parent.span.StartTime && childEndTime <= parentEndTime {
		// child already fits within the parent span, do not adjust
		return 0
	}
	// Assume that network latency is equally split between req and res.
	latency := (parentDuration - childDuration) / 2
	// Goal: parentStartTime + latency = childStartTime + adjustment
	return int64(parent.span.StartTime) + int64(latency) - int64(child.span.StartTime)
}

func (a *clockSkewAdjuster) adjustTimestamps(n *node, skew clockSkew) {
	// because timestamps are unsigned int64, treat negative delta separately
	if skew.delta > 0 {
		delta := uint64(skew.delta)
		n.span.StartTime += delta
		for i := range n.span.Logs {
			n.span.Logs[i].Timestamp += delta
		}
	} else if skew.delta < 0 {
		delta := uint64(-skew.delta)
		n.span.StartTime -= delta
		for i := range n.span.Logs {
			n.span.Logs[i].Timestamp -= delta
		}
	}
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"slices"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	exprproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

// fingerprintSize is how much of the hash a token carries. The fingerprint tells a token minted
// for one query from one minted for another; it is not a secret, so half of a SHA-256 is plenty
// and keeps the token short.
const fingerprintSize = 16

// Fingerprint hashes the parameters of the search that select and order its results, which is
// every field except the page bound and the token. A PageToken continues the ordering of exactly
// one query, and these fields are what define it (RFC 0014 §3.2). The query is the one the Reader
// receives, after the query service has settled it, so a predicate an interceptor added is part
// of what the token is bound to.
func (q TraceQueryParams) Fingerprint() ([]byte, error) {
	h := newHasher("trace")
	h.string(q.ServiceName)
	h.string(q.OperationName)
	h.attributes(q.Attributes)
	h.time(q.StartTimeMin)
	h.time(q.StartTimeMax)
	h.int64(int64(q.DurationMin))
	h.int64(int64(q.DurationMax))
	if err := h.filter(q.Filter); err != nil {
		return nil, err
	}
	return h.sum(), nil
}

// Fingerprint is TraceQueryParams.Fingerprint for a span search, whose selecting parameters are
// the time range and the filter (RFC 0016 §4.3).
func (q SpanQueryParams) Fingerprint() ([]byte, error) {
	h := newHasher("span")
	h.time(q.StartTimeMin)
	h.time(q.StartTimeMax)
	if err := h.filter(q.Filter); err != nil {
		return nil, err
	}
	return h.sum(), nil
}

// hasher writes each field as its length followed by its bytes, so that two queries whose
// fields differ only in where one string ends and the next begins hash differently.
type hasher struct {
	h hash.Hash
}

func newHasher(kind string) hasher {
	h := hasher{h: sha256.New()}
	h.string(kind)
	return h
}

func (h hasher) bytes(b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	h.h.Write(n[:])
	h.h.Write(b)
}

func (h hasher) string(s string) {
	h.bytes([]byte(s))
}

func (h hasher) int64(v int64) {
	// A hash's Write never fails, so the error binary.Write reports is not a case here.
	_ = binary.Write(h.h, binary.BigEndian, v)
}

func (h hasher) time(t time.Time) {
	if t.IsZero() {
		h.int64(0)
		return
	}
	h.int64(t.UnixNano())
}

// attributes hashes the map in key order, since pcommon.Map keeps insertion order and two
// requests may list the same tags differently.
func (h hasher) attributes(attrs pcommon.Map) {
	if attrs == (pcommon.Map{}) {
		h.int64(0)
		return
	}
	keys := make([]string, 0, attrs.Len())
	for k := range attrs.All() {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h.int64(int64(len(keys)))
	for _, k := range keys {
		v, _ := attrs.Get(k)
		h.string(k)
		h.string(v.Type().String())
		h.string(v.AsString())
	}
}

// filter hashes the wire encoding of the filter's canonical form: the tree with the operands
// of every commutative operator, and the values of every list, in a fixed order. Two filters
// that differ only in such an order select the same spans, and the same request can arrive
// with its predicates permuted, because the attributes it was expanded from have no order on
// the wire. Every filter reaching a reader has passed FinalizeFilter, so an
// encoding failure here is a term the wire cannot carry and is reported rather than hashed.
func (h hasher) filter(filter *expression.Call) error {
	if filter == nil {
		h.int64(0)
		return nil
	}
	b, err := encodeCanonical(filter)
	if err != nil {
		return fmt.Errorf("cannot fingerprint the query filter: %w", err)
	}
	h.bytes(b)
	return nil
}

// encodeCanonical returns the wire encoding of the call's canonical form. The generated
// message has no maps, so the encoding of a given tree is deterministic.
func encodeCanonical(call *expression.Call) ([]byte, error) {
	canonical, err := canonicalize(call)
	if err != nil {
		return nil, err
	}
	msg, err := exprproto.ToProto(canonical)
	if err != nil {
		return nil, err
	}
	return msg.Marshal()
}

// canonicalElements returns a copy of a list's elements with each one that is compared against a
// time field respelled the way its typed constant is encoded: an instant in UTC and a duration
// in Go's own form. A list has no typed node, so its elements stay text (see checkMembership),
// and one that does not parse is kept as it is, since a finalized filter has none.
func canonicalElements(call *expression.Call, list *expression.List) []string {
	values := slices.Clone(list.Values)
	ref, ok := call.Args[0].(*expression.FieldRef)
	if !ok || ref == nil {
		return values
	}
	field, ok := expression.LookupField(ref.Level, ref.Name)
	if !ok {
		return values
	}
	for i, element := range values {
		switch field.Type {
		case expression.FieldTypeTimestamp:
			if v, err := time.Parse(time.RFC3339Nano, element); err == nil {
				values[i] = v.UTC().Format(time.RFC3339Nano)
			}
		case expression.FieldTypeDuration:
			if v, err := time.ParseDuration(element); err == nil {
				values[i] = v.String()
			}
		default:
			// Every other field type has one spelling per value already.
		}
	}
	return values
}

// canonicalize returns a copy of the call with the operands of `and` and `or`, and the values
// of every list, sorted by their own canonical encodings, and every time constant in one
// spelling, since the wire carries an instant with whatever offset the client used and a
// duration in whatever unit. Operands of every other operator keep their order, since it is
// part of the operator's meaning.
func canonicalize(call *expression.Call) (*expression.Call, error) {
	out := &expression.Call{Op: call.Op, Args: make([]expression.Expression, len(call.Args))}
	for i, arg := range call.Args {
		switch term := arg.(type) {
		case *expression.Call:
			if term == nil {
				out.Args[i] = arg
				continue
			}
			nested, err := canonicalize(term)
			if err != nil {
				return nil, err
			}
			out.Args[i] = nested
		case *expression.List:
			if term == nil {
				out.Args[i] = arg
				continue
			}
			values := canonicalElements(call, term)
			slices.Sort(values)
			out.Args[i] = &expression.List{Values: values, Type: term.Type}
		case *expression.TimestampValue:
			if term == nil {
				out.Args[i] = arg
				continue
			}
			out.Args[i] = &expression.TimestampValue{Value: term.Value.UTC()}
		default:
			out.Args[i] = arg
		}
	}
	if call.Op != expression.OpAnd && call.Op != expression.OpOr {
		return out, nil
	}
	// Each operand is ordered by its own encoding, which is well defined for a canonical
	// subtree and for a leaf alike; wrapping a leaf in a call is what gives it one.
	keys := make([][]byte, len(out.Args))
	for i, arg := range out.Args {
		wrapped := &expression.Call{Args: []expression.Expression{arg}}
		msg, err := exprproto.ToProto(wrapped)
		if err != nil {
			return nil, err
		}
		if keys[i], err = msg.Marshal(); err != nil {
			return nil, err
		}
	}
	order := make([]int, len(out.Args))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return bytes.Compare(keys[a], keys[b]) })
	sorted := make([]expression.Expression, len(out.Args))
	for i, from := range order {
		sorted[i] = out.Args[from]
	}
	out.Args = sorted
	return out, nil
}

func (h hasher) sum() []byte {
	return h.h.Sum(nil)[:fingerprintSize]
}

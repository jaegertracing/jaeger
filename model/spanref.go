// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// ChildOf span reference type describes a reference to a parent span
	// that depends on the response from the current (child) span
	ChildOf = SpanRefType_CHILD_OF

	// FollowsFrom span reference type describes a reference to a "parent" span
	// that does not depend on the response from the current (child) span
	FollowsFrom = SpanRefType_FOLLOWS_FROM
)

// MaybeAddParentSpanID adds non-zero parentSpanID to refs as a child-of reference.
// We no longer store ParentSpanID in the domain model, but the data in the database
// or other formats might still have these IDs without representing them in the References,
// so this converts parent IDs to canonical reference format.
var MaybeAddParentSpanID = modelv1.MaybeAddParentSpanID

// NewChildOfRef creates a new child-of span reference.
var NewChildOfRef = modelv1.NewChildOfRef

// NewFollowsFromRef creates a new follows-from span reference.
var NewFollowsFromRef = modelv1.NewFollowsFromRef

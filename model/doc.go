// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package model describes the internal data model for Trace and Span
package model

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Establish dependency on jaeger-idl.
var _ jsonpb.JSONPBUnmarshaler = (*model.SpanID)(nil)

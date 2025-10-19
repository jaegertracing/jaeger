// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

type Operation struct {
	Name     string `ch:"name"`
	SpanKind string `ch:"span_kind"`
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// Hashable interface is for type that can participate in a hash computation
// by writing their data into io.Writer, which is usually an instance of hash.Hash.
type Hashable = modelv1.Hashable

// HashCode calculates a FNV-1a hash code for a Hashable object.
func HashCode(o Hashable) (uint64, error) {
	return modelv1.HashCode(o)
}

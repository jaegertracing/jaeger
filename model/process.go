// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// NewProcess creates a new Process for given serviceName and tags.
// The tags are sorted in place and kept in the same array/slice,
// in order to store the Process in a canonical form that is relied
// upon by the Equal and Hash functions.
var NewProcess = modelv1.NewProcess

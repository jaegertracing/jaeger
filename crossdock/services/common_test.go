// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTracerServiceName(t *testing.T) {
	assert.Equal(t, "crossdock-go", getTracerServiceName("go"))
}

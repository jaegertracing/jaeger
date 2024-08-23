// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigV2(t *testing.T) {
	cfg := DefaultConfigV2()
	assert.NotEmpty(t, cfg.Timeout)
}

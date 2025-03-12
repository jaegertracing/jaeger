// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package httpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestHTTPServer(t *testing.T) {
	s := NewHTTPServer(":1", nil, nil, zap.NewNop())
	assert.NotNil(t, s)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

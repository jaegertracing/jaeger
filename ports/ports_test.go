// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestPortToHostPort(t *testing.T) {
	assert.Equal(t, ":42", PortToHostPort(42))
}

func TestFormatHostPort(t *testing.T) {
	assert.Equal(t, ":42", FormatHostPort("42"))
	assert.Equal(t, ":831", FormatHostPort(":831"))
	assert.Equal(t, "", FormatHostPort(""))
	assert.Equal(t, "localhost:42", FormatHostPort("localhost:42"))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

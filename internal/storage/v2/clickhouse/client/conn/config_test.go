// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package conn

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestDefaultConfig(t *testing.T) {
	expected := Configuration{
		Address:  []string{"127.0.0.1:9000"},
		Database: "default",
		Username: "default",
		Password: "default",
	}
	actual := DefaultConfig()
	assert.NotNil(t, actual)
	assert.Equal(t, expected, actual)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

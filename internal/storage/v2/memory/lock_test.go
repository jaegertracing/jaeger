// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquire(t *testing.T) {
	l := &Lock{}
	ok, err := l.Acquire("resource", time.Duration(1))
	assert.True(t, ok)
	require.NoError(t, err)
}

func TestForfeit(t *testing.T) {
	l := &Lock{}
	ok, err := l.Forfeit("resource")
	assert.True(t, ok)
	require.NoError(t, err)
}

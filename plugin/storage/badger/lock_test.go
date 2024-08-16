// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquire(t *testing.T) {
	l := &lock{}
	ok, err := l.Acquire("resource", time.Duration(1))
	assert.True(t, ok)
	require.NoError(t, err)
}

func TestForfeit(t *testing.T) {
	l := &lock{}
	ok, err := l.Forfeit("resource")
	assert.True(t, ok)
	require.NoError(t, err)
}

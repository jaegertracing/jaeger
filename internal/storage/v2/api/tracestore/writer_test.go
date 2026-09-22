// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectedSpansError(t *testing.T) {
	backend := errors.New("2 of 3 bulk items rejected")
	err := fmt.Errorf("write failed: %w", &RejectedSpansError{Unidentified: 1, Err: backend})

	var rejected *RejectedSpansError
	require.ErrorAs(t, err, &rejected, "the type survives wrapping")
	assert.Equal(t, 1, rejected.Unidentified)
	assert.Equal(t, "2 of 3 bulk items rejected", rejected.Error(), "the message is the backend's")
	assert.ErrorIs(t, err, backend, "the backend error stays reachable")
}

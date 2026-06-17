// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestNewFactory(t *testing.T) {
	require.NotNil(t, NewFactory())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

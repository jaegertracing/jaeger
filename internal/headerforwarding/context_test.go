// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

func makeHeader(httpName string) headerforwarding.ForwardedHeader {
	return headerforwarding.ForwardedHeader{HTTPName: httpName, Role: headerforwarding.RoleUsername}
}

func TestContextRoundtrip(t *testing.T) {
	hdr := makeHeader("x-user")

	// Verify storage and retrieval of a single captured header.
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: &hdr, Value: "alice"},
	})
	got := headerforwarding.CapturedFromContext(ctx)
	require.Len(t, got, 1)
	assert.Equal(t, "alice", got[0].Value)
	assert.Equal(t, &hdr, got[0].Header)
}

func TestCapturedFromContext_Missing(t *testing.T) {
	assert.Nil(t, headerforwarding.CapturedFromContext(context.Background()))
}

func TestContextWithCaptured_Empty(t *testing.T) {
	ctx := headerforwarding.ContextWithCaptured(context.Background(), nil)
	assert.Nil(t, headerforwarding.CapturedFromContext(ctx))

	ctx2 := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{})
	assert.Nil(t, headerforwarding.CapturedFromContext(ctx2))
}

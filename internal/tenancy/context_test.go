// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testContextKey string

func TestContextTenantHandling(t *testing.T) {
	ctxWithTenant := WithTenant(context.Background(), "tenant1")
	assert.Equal(t, "tenant1", GetTenant(ctxWithTenant))
}

func TestContextPreserved(t *testing.T) {
	key := testContextKey("expected-key")
	val := "expected-value"
	ctxWithValue := context.WithValue(context.Background(), key, val)
	ctxWithTenant := WithTenant(ctxWithValue, "tenant1")
	assert.Equal(t, "tenant1", GetTenant(ctxWithTenant))
	assert.Equal(t, val, ctxWithTenant.Value(key))
}

func TestNoTenant(t *testing.T) {
	// If no tenant in context, GetTenant should return the empty string
	assert.Equal(t, "", GetTenant(context.Background()))
}

func TestImpossibleTenantType(t *testing.T) {
	// If the tenant is not a string, GetTenant should return the empty string
	ctxWithIntTenant := context.WithValue(context.Background(), tenantKey, -1)
	assert.Equal(t, "", GetTenant(ctxWithIntTenant))
}

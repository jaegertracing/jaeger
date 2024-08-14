// Copyright (c) 2022 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

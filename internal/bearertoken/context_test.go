// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetBearerToken(t *testing.T) {
	const token = "blah"
	ctx := context.Background()
	ctx = ContextWithBearerToken(ctx, token)
	contextToken, ok := GetBearerToken(ctx)
	assert.True(t, ok)
	assert.Equal(t, token, contextToken)
}

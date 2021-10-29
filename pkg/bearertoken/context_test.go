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
	assert.Equal(t, contextToken, token)
}

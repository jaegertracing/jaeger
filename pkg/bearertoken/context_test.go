package bearertoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetBearerToken(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsIm5hbWUiOiJKb2huIERvZSIsImlhdCI"
	ctx := context.Background()
	ctx = ContextWithBearerToken(ctx, token)
	contextToken, ok := GetBearerToken(ctx)
	assert.True(t, ok)
	assert.Equal(t, contextToken, token)
}

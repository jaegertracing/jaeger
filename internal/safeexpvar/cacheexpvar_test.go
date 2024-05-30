package safeexpvar

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetExpvarInt(t *testing.T) {
	// Test with a new variable
	name := "metrics-test-1"
	value := int64(42)

	SetExpvarInt(name, value)

	// Retrieve the variable and check its value
	v := expvar.Get(name)
	assert.NotNil(t, v, "expected variable %s to be created", name)
	expInt, ok := v.(*expvar.Int)
	assert.True(t, ok, "expected variable %s to be of type *expvar.Int", name)
	assert.Equal(t, value, expInt.Value(), "expected variable %s value to be %d", name, value)
}

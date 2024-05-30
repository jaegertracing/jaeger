package safeexpvar

import (
	"expvar"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetExpvarInt(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
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

	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineDiff := finalGoroutines - initialGoroutines

	assert.Equal(t, 0, goroutineDiff, "goroutine leak detected: initial goroutines %d, final goroutines %d", initialGoroutines, finalGoroutines)
}

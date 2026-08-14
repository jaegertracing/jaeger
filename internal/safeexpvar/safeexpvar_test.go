// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package safeexpvar

import (
	"expvar"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestSetInt(t *testing.T) {
	// Test with a new variable
	name := "metrics-test-1"
	value := int64(42)

	SetInt(name, value)

	// Retrieve the variable and check its value
	v := expvar.Get(name)
	assert.NotNil(t, v, "expected variable %s to be created", name)
	expInt, ok := v.(*expvar.Int)
	require.True(t, ok, "expected variable %s to be of type *expvar.Int", name)
	assert.Equal(t, value, expInt.Value())
}

// Regression: concurrent first use of the same name used to let two goroutines
// both see nil from expvar.Get and both call expvar.NewInt, panicking the second
// with "Reuse of exported var name". Run with -race for the full signal.
func TestSetIntConcurrentFirstUse(t *testing.T) {
	name := "metrics-test-concurrent"
	const goroutines = 32

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	for i := range goroutines {
		go func() {
			defer done.Done()
			start.Wait() // release everyone at once to widen the race window
			assert.NotPanics(t, func() { SetInt(name, int64(i)) })
		}()
	}

	start.Done()
	done.Wait()

	v := expvar.Get(name)
	require.NotNil(t, v, "expected variable %s to be created exactly once", name)
	expInt, ok := v.(*expvar.Int)
	require.True(t, ok, "expected variable %s to be of type *expvar.Int", name)
	// The winning writer is nondeterministic; only publication matters here.
	assert.Less(t, expInt.Value(), int64(goroutines))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

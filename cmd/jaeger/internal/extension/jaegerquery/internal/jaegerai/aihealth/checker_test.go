// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aihealth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestChecker(t *testing.T, check func(ctx context.Context) error) *Checker {
	t.Helper()
	r, err := New(Config{
		AgentURL: "ws://test.invalid:0",
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	})
	require.NoError(t, err)
	r.check = check
	return r
}

func TestNew_RequiresAgentURL(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
}

func TestChecker_InitialStateIsFalse(t *testing.T) {
	r := newTestChecker(t, func(context.Context) error { return errors.New("unused") })
	require.False(t, r.Current(), "before Start, Current must be false")
}

func TestChecker_FlipsToTrueOnHealthySidecar(t *testing.T) {
	r := newTestChecker(t, func(context.Context) error { return nil })
	r.Start(t.Context())
	defer r.Stop()

	require.Eventually(t, r.Current, time.Second, 10*time.Millisecond,
		"Current() should flip to true once a check succeeds")
}

func TestChecker_FlipsBackToFalseWhenSidecarDies(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	check := func(context.Context) error {
		if healthy.Load() {
			return nil
		}
		return errors.New("down")
	}
	r := newTestChecker(t, check)
	r.Start(t.Context())
	defer r.Stop()

	require.Eventually(t, r.Current, time.Second, 10*time.Millisecond,
		"Current() should flip to true while sidecar is healthy")

	healthy.Store(false)
	require.Eventually(t, func() bool { return !r.Current() },
		time.Second, 10*time.Millisecond,
		"Current() should flip back to false once sidecar stops responding")
}

func TestChecker_StaysFalseWhenSidecarNeverResponds(t *testing.T) {
	r := newTestChecker(t, func(context.Context) error { return errors.New("always down") })

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	<-ctx.Done()
	require.False(t, r.Current())
}

func TestChecker_StopWithoutStartIsNoOp(t *testing.T) {
	r := newTestChecker(t, func(context.Context) error { return nil })
	r.Stop() // must not block or panic
}

func TestChecker_StartTwicePanics(t *testing.T) {
	r := newTestChecker(t, func(context.Context) error { return nil })
	r.Start(t.Context())
	defer r.Stop()
	require.Panics(t, func() { r.Start(t.Context()) })
}

func TestChecker_CheckTimeoutIsApplied(t *testing.T) {
	var observed atomic.Int64
	check := func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("no deadline")
		}
		observed.Store(int64(time.Until(deadline)))
		return nil
	}

	r, err := New(Config{
		AgentURL: "ws://test.invalid:0",
		Interval: 50 * time.Millisecond,
		Timeout:  200 * time.Millisecond,
	})
	require.NoError(t, err)
	r.check = check

	r.Start(t.Context())
	defer r.Stop()

	require.Eventually(t, func() bool {
		d := time.Duration(observed.Load())
		return d > 0 && d <= 200*time.Millisecond
	}, time.Second, 10*time.Millisecond, "check context must inherit the configured Timeout")
}

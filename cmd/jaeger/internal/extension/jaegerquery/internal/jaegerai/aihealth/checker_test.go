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
	"go.uber.org/zap"
)

func newTestChecker(check func(ctx context.Context) error) *Checker {
	return &Checker{
		Check:    check,
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
		Logger:   zap.NewNop(),
	}
}

func TestChecker_InitialStateIsFalse(t *testing.T) {
	c := newTestChecker(func(context.Context) error { return errors.New("unused") })
	require.False(t, c.Current(), "before Start, Current must be false")
}

func TestChecker_FlipsToTrueOnHealthyCheck(t *testing.T) {
	c := newTestChecker(func(context.Context) error { return nil })
	c.Start(t.Context())
	defer c.Stop()

	require.Eventually(t, c.Current, time.Second, 10*time.Millisecond,
		"Current() should flip to true once a check succeeds")
}

func TestChecker_FlipsBackToFalseWhenCheckStartsFailing(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	c := newTestChecker(func(context.Context) error {
		if healthy.Load() {
			return nil
		}
		return errors.New("down")
	})
	c.Start(t.Context())
	defer c.Stop()

	require.Eventually(t, c.Current, time.Second, 10*time.Millisecond,
		"Current() should flip to true while check succeeds")

	healthy.Store(false)
	require.Eventually(t, func() bool { return !c.Current() },
		time.Second, 10*time.Millisecond,
		"Current() should flip back to false once check starts failing")
}

func TestChecker_StaysFalseWhenCheckAlwaysFails(t *testing.T) {
	c := newTestChecker(func(context.Context) error { return errors.New("always down") })

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	c.Start(ctx)
	defer c.Stop()

	<-ctx.Done()
	require.False(t, c.Current())
}

func TestChecker_StopWithoutStartIsNoOp(_ *testing.T) {
	c := newTestChecker(func(context.Context) error { return nil })
	c.Stop() // must not block or panic
}

func TestChecker_StartTwicePanics(t *testing.T) {
	c := newTestChecker(func(context.Context) error { return nil })
	c.Start(t.Context())
	defer c.Stop()
	require.Panics(t, func() { c.Start(t.Context()) })
}

func TestChecker_CheckTimeoutIsApplied(t *testing.T) {
	var observed atomic.Int64
	c := &Checker{
		Check: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				return errors.New("no deadline")
			}
			observed.Store(int64(time.Until(deadline)))
			return nil
		},
		Interval: 50 * time.Millisecond,
		Timeout:  200 * time.Millisecond,
		Logger:   zap.NewNop(),
	}
	c.Start(t.Context())
	defer c.Stop()

	require.Eventually(t, func() bool {
		d := time.Duration(observed.Load())
		return d > 0 && d <= 200*time.Millisecond
	}, time.Second, 10*time.Millisecond, "check context must inherit the configured Timeout")
}

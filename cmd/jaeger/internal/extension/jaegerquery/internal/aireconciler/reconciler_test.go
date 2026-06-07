// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aireconciler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestReconciler(t *testing.T, probe func(ctx context.Context) error) *Reconciler {
	t.Helper()
	r, err := New(Config{
		AgentURL: "ws://test.invalid:0",
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	})
	require.NoError(t, err)
	r.probe = probe
	return r
}

func TestNew_RequiresAgentURL(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
}

func TestReconciler_InitialStateIsFalse(t *testing.T) {
	r := newTestReconciler(t, func(context.Context) error { return errors.New("unused") })
	require.False(t, r.Current(), "before Start, Current must be false")
}

func TestReconciler_FlipsToTrueAndNotifies(t *testing.T) {
	notified := make(chan struct{}, 4)
	r := newTestReconciler(t, func(context.Context) error { return nil })
	r.Subscribe(func() { notified <- struct{}{} })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified within 1s")
	}
	require.True(t, r.Current())
}

func TestReconciler_FlipsBackToFalse(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	probe := func(context.Context) error {
		if healthy.Load() {
			return nil
		}
		return errors.New("down")
	}

	notifications := make(chan struct{}, 16)
	r := newTestReconciler(t, probe)
	r.Subscribe(func() { notifications <- struct{}{} })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	// Wait for the first flip to true.
	waitFor(t, notifications, time.Second)
	require.True(t, r.Current())

	// Knock the sidecar over and wait for the flip back to false.
	healthy.Store(false)
	waitFor(t, notifications, time.Second)
	require.False(t, r.Current())
}

func TestReconciler_NoNotifyWhenStateUnchanged(t *testing.T) {
	var count atomic.Int32
	r := newTestReconciler(t, func(context.Context) error { return errors.New("always down") })
	r.Subscribe(func() { count.Add(1) })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	<-ctx.Done()
	// Initial state is false; failing probes keep it false. Subscribers
	// must not fire because there's nothing to report.
	require.Zero(t, count.Load(), "subscriber should not be notified when state stays false")
}

func TestReconciler_MultipleSubscribersAllNotified(t *testing.T) {
	var a, b atomic.Int32
	r := newTestReconciler(t, func(context.Context) error { return nil })
	r.Subscribe(func() { a.Add(1) })
	r.Subscribe(func() { b.Add(1) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	require.Eventually(t, func() bool { return a.Load() >= 1 && b.Load() >= 1 },
		time.Second, 10*time.Millisecond, "both subscribers must be notified")
}

func TestReconciler_NilSubscriberIgnored(t *testing.T) {
	r := newTestReconciler(t, func(context.Context) error { return nil })
	r.Subscribe(nil) // must not panic
	r.notify()       // must not panic
}

func TestReconciler_StopWithoutStartIsNoOp(t *testing.T) {
	r := newTestReconciler(t, func(context.Context) error { return nil })
	r.Stop() // must not block or panic
}

func TestReconciler_StartTwicePanics(t *testing.T) {
	r := newTestReconciler(t, func(context.Context) error { return nil })
	ctx := t.Context()
	r.Start(ctx)
	defer r.Stop()
	require.Panics(t, func() { r.Start(ctx) })
}

func TestReconciler_ProbeTimeoutIsApplied(t *testing.T) {
	var observed atomic.Int64
	probe := func(ctx context.Context) error {
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
	r.probe = probe

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Start(ctx)
	defer r.Stop()

	require.Eventually(t, func() bool {
		d := time.Duration(observed.Load())
		return d > 0 && d <= 200*time.Millisecond
	}, time.Second, 10*time.Millisecond, "probe context must inherit the configured Timeout")
}

// waitFor consumes one item from the channel or fails the test on timeout.
func waitFor(t *testing.T, ch <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatal("timeout waiting for channel")
	}
}

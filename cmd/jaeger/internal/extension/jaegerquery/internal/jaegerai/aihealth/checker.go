// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package aihealth periodically runs a caller-supplied health check and
// tracks whether the latest result was a success. Used by the query
// extension to drive the backendCapabilities.aiAssistant flag from a
// liveness check against the AI sidecar; see acp_check.go for the concrete
// check function.
package aihealth

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Checker runs Check periodically and tracks whether the latest call
// succeeded. Callers populate the public fields, then call Start; Current()
// reports the latest state and is safe to call from any goroutine.
//
// All public fields are required and must be valid before Start: Check
// non-nil, Interval and Timeout positive (zero or negative trips the
// underlying time.NewTicker / context.WithTimeout into a panic or instant
// failure), Logger non-nil. Validation is the caller's responsibility.
type Checker struct {
	// Check is the single-shot health check the loop invokes on each tick.
	// A nil error means healthy; any non-nil error means unhealthy.
	Check    func(ctx context.Context) error
	Interval time.Duration
	Timeout  time.Duration
	Logger   *zap.Logger

	current atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

// Current returns the most recently observed health state. Initial value is
// false until the first check completes.
func (c *Checker) Current() bool { return c.current.Load() }

// Start launches the checker's background goroutine. The first check runs
// immediately so callers see the truth as soon as possible; subsequent
// checks are spaced by Interval. Start may be called only once per Checker.
func (c *Checker) Start(ctx context.Context) {
	if c.done != nil {
		panic("aihealth: Start called twice")
	}
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	go c.run(ctx)
}

// Stop signals the background goroutine to exit and waits for it. Safe to
// call multiple times; safe to call before Start (no-op).
func (c *Checker) Stop() {
	if c.done == nil {
		return
	}
	c.cancel()
	<-c.done
}

func (c *Checker) run(ctx context.Context) {
	defer close(c.done)

	c.runOnce(ctx)

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runOnce(ctx)
		}
	}
}

func (c *Checker) runOnce(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	err := c.Check(checkCtx)
	healthy := err == nil
	if err != nil {
		c.Logger.Debug("AI health check failed", zap.Error(err))
	}
	if prev := c.current.Swap(healthy); prev != healthy {
		c.Logger.Info("AI health state changed", zap.Bool("healthy", healthy))
	}
}

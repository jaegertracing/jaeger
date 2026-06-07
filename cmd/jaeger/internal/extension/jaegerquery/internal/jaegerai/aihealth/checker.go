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
	"errors"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config bundles the inputs the checker needs. Check is mandatory;
// Interval and Timeout default to AIConfig's documented defaults if zero.
type Config struct {
	// Check is the single-shot health check the checker invokes on each
	// tick. A nil error means healthy; any non-nil error means unhealthy.
	Check    func(ctx context.Context) error
	Interval time.Duration
	Timeout  time.Duration
	Logger   *zap.Logger
}

// Checker runs Config.Check periodically and tracks whether the latest call
// succeeded. Callers read the latest state via Current(), which is safe to
// call from any goroutine.
//
// The zero Checker is not usable; construct via New.
type Checker struct {
	check    func(ctx context.Context) error
	interval time.Duration
	timeout  time.Duration
	logger   *zap.Logger

	current atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Checker. Config.Check must be non-nil.
func New(cfg Config) (*Checker, error) {
	if cfg.Check == nil {
		return nil, errors.New("aihealth: Config.Check must be non-nil")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return &Checker{
		check:    cfg.Check,
		interval: cfg.Interval,
		timeout:  cfg.Timeout,
		logger:   cfg.Logger,
	}, nil
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

	ticker := time.NewTicker(c.interval)
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
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	err := c.check(checkCtx)
	healthy := err == nil
	if err != nil {
		c.logger.Debug("AI health check failed", zap.Error(err))
	}
	if prev := c.current.Swap(healthy); prev != healthy {
		c.logger.Info("AI health state changed", zap.Bool("healthy", healthy))
	}
}

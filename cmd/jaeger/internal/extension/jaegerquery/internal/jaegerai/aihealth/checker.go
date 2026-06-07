// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package aihealth periodically checks the AI sidecar to determine whether
// the chat surface should be advertised to the UI as a backend capability.
//
// The checker runs only when the jaeger_query.ai config block is present.
// When it is absent, the static handler skips construction entirely and the
// advertised aiAssistant capability stays at its initial false value.
package aihealth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/internal/version"
)

// Config bundles the inputs the checker needs. AgentURL must be non-empty;
// Interval and Timeout default to AIConfig's documented defaults if zero.
type Config struct {
	AgentURL string
	Interval time.Duration
	Timeout  time.Duration
	Logger   *zap.Logger
}

// Checker periodically checks an ACP sidecar and tracks whether it is
// currently reachable. Callers read the latest state via Current(), which
// is safe to call from any goroutine.
//
// The zero Checker is not usable; construct via New.
type Checker struct {
	agentURL string
	interval time.Duration
	timeout  time.Duration
	logger   *zap.Logger

	// check is the function used to perform a single reachability check.
	// Defaults to ACP `initialize` over WebSocket; overridable for tests.
	check func(ctx context.Context) error

	current atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Checker. AgentURL must be non-empty.
func New(cfg Config) (*Checker, error) {
	if cfg.AgentURL == "" {
		return nil, errors.New("aihealth: AgentURL must be non-empty")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	r := &Checker{
		agentURL: cfg.AgentURL,
		interval: cfg.Interval,
		timeout:  cfg.Timeout,
		logger:   cfg.Logger,
	}
	r.check = r.acpCheck
	return r, nil
}

// Current returns the most recently observed reachability state. Initial
// value is false until the first check completes.
func (r *Checker) Current() bool { return r.current.Load() }

// Start launches the checker's background goroutine. The first check runs
// immediately so the UI lights up as soon as the operator brings both
// processes online; subsequent checks are spaced by Interval. Start may be
// called only once per Checker.
func (r *Checker) Start(ctx context.Context) {
	if r.done != nil {
		panic("aihealth: Start called twice")
	}
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx)
}

// Stop signals the background goroutine to exit and waits for it. Safe to
// call multiple times; safe to call before Start (no-op).
func (r *Checker) Stop() {
	if r.done == nil {
		return
	}
	r.cancel()
	<-r.done
}

func (r *Checker) run(ctx context.Context) {
	defer close(r.done)

	r.runOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

func (r *Checker) runOnce(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	err := r.check(checkCtx)
	healthy := err == nil
	if err != nil {
		r.logger.Debug("AI sidecar health check failed", zap.String("agent_url", r.agentURL), zap.Error(err))
	}
	prev := r.current.Swap(healthy)
	if prev != healthy {
		r.logger.Info(
			"AI sidecar reachability changed",
			zap.String("agent_url", r.agentURL),
			zap.Bool("reachable", healthy),
		)
	}
}

// acpCheck performs one ACP `initialize` round-trip against the sidecar.
// Any transport-level or protocol-level error counts as unreachable.
func (r *Checker) acpCheck(ctx context.Context) error {
	adapter, err := jaegerai.DialWsAdapter(ctx, r.agentURL, r.logger)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer adapter.Close()

	conn := acp.NewConnection(noopMethodHandler, adapter, adapter)

	if _, err := acp.SendRequest[acp.InitializeResponse](conn, ctx, acp.AgentMethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-check",
			Version: version.Get().GitVersion,
		},
	}); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	return nil
}

// noopMethodHandler returns MethodNotFound for every inbound call. The
// checker only sends an `initialize` request to the sidecar and immediately
// closes the connection — the sidecar should not send any client-bound
// calls in that window, but if it does we refuse them rather than crash.
func noopMethodHandler(_ context.Context, method string, _ json.RawMessage) (any, *acp.RequestError) {
	return nil, acp.NewMethodNotFound(method)
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package aireconciler periodically probes the AI sidecar to determine whether
// the chat surface should be advertised to the UI as a backend capability.
//
// The reconciler runs only when the jaeger_query.ai config block is present.
// When it is absent, the static handler skips construction entirely and the
// advertised aiAssistant capability stays at its initial false value.
package aireconciler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/internal/version"
)

// Config bundles the inputs the reconciler needs. AgentURL must be non-empty;
// Interval and Timeout default to AIConfig's documented defaults if zero.
type Config struct {
	AgentURL string
	Interval time.Duration
	Timeout  time.Duration
	Logger   *zap.Logger
}

// Reconciler periodically probes an ACP sidecar and tracks whether it is
// currently reachable. Subscribers are notified when the reachability flips.
//
// The zero Reconciler is not usable; construct via New.
type Reconciler struct {
	agentURL string
	interval time.Duration
	timeout  time.Duration
	logger   *zap.Logger

	// probe is the function used to perform a single reachability check.
	// Defaults to acp `initialize` over WebSocket; overridable for tests.
	probe func(ctx context.Context) error

	current atomic.Bool

	mu   sync.Mutex
	subs []func()

	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Reconciler. AgentURL must be non-empty.
func New(cfg Config) (*Reconciler, error) {
	if cfg.AgentURL == "" {
		return nil, errors.New("aireconciler: AgentURL must be non-empty")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	r := &Reconciler{
		agentURL: cfg.AgentURL,
		interval: cfg.Interval,
		timeout:  cfg.Timeout,
		logger:   cfg.Logger,
	}
	r.probe = r.acpProbe
	return r, nil
}

// Current returns the most recently observed reachability state. Initial
// value is false until the first probe completes.
func (r *Reconciler) Current() bool { return r.current.Load() }

// Subscribe registers a callback fired (synchronously, in the reconciler's
// goroutine) whenever Current() flips. Callbacks must not block.
func (r *Reconciler) Subscribe(fn func()) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	r.subs = append(r.subs, fn)
	r.mu.Unlock()
}

// Start launches the reconciler's background goroutine. The first probe runs
// immediately so the UI lights up as soon as the operator brings both
// processes online; subsequent probes are spaced by Interval. Start may be
// called only once per Reconciler.
func (r *Reconciler) Start(ctx context.Context) {
	if r.done != nil {
		panic("aireconciler: Start called twice")
	}
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx)
}

// Stop signals the background goroutine to exit and waits for it. Safe to
// call multiple times; safe to call before Start (no-op).
func (r *Reconciler) Stop() {
	if r.done == nil {
		return
	}
	r.cancel()
	<-r.done
}

func (r *Reconciler) run(ctx context.Context) {
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

func (r *Reconciler) runOnce(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	err := r.probe(probeCtx)
	healthy := err == nil
	if err != nil {
		r.logger.Debug("AI sidecar probe failed", zap.String("agent_url", r.agentURL), zap.Error(err))
	}
	prev := r.current.Swap(healthy)
	if prev != healthy {
		r.logger.Info(
			"AI sidecar reachability changed",
			zap.String("agent_url", r.agentURL),
			zap.Bool("reachable", healthy),
		)
		r.notify()
	}
}

func (r *Reconciler) notify() {
	r.mu.Lock()
	subs := make([]func(), len(r.subs))
	copy(subs, r.subs)
	r.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// acpProbe performs one ACP `initialize` round-trip against the sidecar.
// Any transport-level or protocol-level error counts as unreachable.
func (r *Reconciler) acpProbe(ctx context.Context) error {
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
			Name:    "jaeger-ai-probe",
			Version: version.Get().GitVersion,
		},
	}); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	return nil
}

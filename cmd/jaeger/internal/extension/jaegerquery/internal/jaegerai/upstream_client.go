// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

// UpstreamCaller abstracts upstream MCP tool execution and listing so that
// both in-process handlers and remote MCP servers can be addressed uniformly.
type UpstreamCaller interface {
	ListTools(ctx context.Context) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, name string, args any) (*mcp.CallToolResult, error)
	Close() error
}

// UpstreamClientOption configures an UpstreamClient.
type UpstreamClientOption func(*UpstreamClient)

// WithUpstreamHTTPClient sets a custom http.Client (e.g. with custom transport).
func WithUpstreamHTTPClient(httpClient *http.Client) UpstreamClientOption {
	return func(c *UpstreamClient) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithDisableStandaloneSSE sets DisableStandaloneSSE on the transport.
func WithDisableStandaloneSSE(disable bool) UpstreamClientOption {
	return func(c *UpstreamClient) {
		c.disableStandaloneSSE = disable
	}
}

// WithUpstreamLogger sets the logger.
func WithUpstreamLogger(logger *zap.Logger) UpstreamClientOption {
	return func(c *UpstreamClient) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithUpstreamRetryInterval sets the initial retry interval for background reconnect.
func WithUpstreamRetryInterval(d time.Duration) UpstreamClientOption {
	return func(c *UpstreamClient) {
		if d > 0 {
			c.retryInterval = d
		}
	}
}

// UpstreamClient manages a connection to an upstream MCP telemetry server.
// It hardens communication by using StreamableClientTransport with
// DisableStandaloneSSE: true, performing a sync-first dial with background retry,
// and handling ErrSessionMissing via transparent re-connect.
type UpstreamClient struct {
	endpoint             string
	httpClient           *http.Client
	disableStandaloneSSE bool
	maxRetries           int
	retryInterval        time.Duration
	logger               *zap.Logger

	mu        sync.RWMutex
	client    *mcp.Client
	session   *mcp.ClientSession
	transport *mcp.StreamableClientTransport
	closed    bool
	bgCtx     context.Context
	cancel    context.CancelFunc
}

var _ UpstreamCaller = (*UpstreamClient)(nil)

// NewUpstreamClient constructs a hardened UpstreamClient.
func NewUpstreamClient(endpoint string, opts ...UpstreamClientOption) *UpstreamClient {
	bgCtx, cancel := context.WithCancel(context.Background())
	c := &UpstreamClient{
		endpoint:             endpoint,
		httpClient:           http.DefaultClient,
		disableStandaloneSSE: true,
		maxRetries:           3,
		retryInterval:        500 * time.Millisecond,
		logger:               zap.NewNop(),
		bgCtx:                bgCtx,
		cancel:               cancel,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.client = mcp.NewClient(
		&mcp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: version.Get().GitVersion,
		},
		nil,
	)
	return c
}

// Connect performs a sync-first dial to the upstream MCP endpoint. If the
// initial dial fails, it returns the error and launches a background retry loop
// that attempts to connect until success or client close.
func (c *UpstreamClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("upstream client closed")
	}
	if c.session != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	err := c.dial(ctx)
	if err == nil {
		return nil
	}

	c.logger.Warn("initial sync dial to upstream MCP failed, starting background retry",
		zap.String("endpoint", c.endpoint),
		zap.Error(err),
	)

	go c.backgroundRetryLoop() //nolint:contextcheck // intentional: retry loop outlives Connect ctx; Close stops it.
	return err
}

func (c *UpstreamClient) dial(ctx context.Context) error {
	transport := &mcp.StreamableClientTransport{
		Endpoint:             c.endpoint,
		HTTPClient:           c.httpClient,
		DisableStandaloneSSE: c.disableStandaloneSSE,
		MaxRetries:           c.maxRetries,
	}

	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = session.Close()
		return errors.New("upstream client closed")
	}
	if c.session != nil {
		_ = c.session.Close()
	}
	c.session = session
	c.transport = transport
	return nil
}

func (c *UpstreamClient) backgroundRetryLoop() {
	c.mu.RLock()
	ctx := c.bgCtx
	c.mu.RUnlock()

	interval := c.retryInterval
	maxInterval := 10 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}

		dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.dial(dialCtx)
		dialCancel()
		if err == nil {
			c.logger.Info("connected to upstream MCP server in background", zap.String("endpoint", c.endpoint))
			return
		}

		interval *= 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}

func isSessionMissingError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, mcp.ErrSessionMissing) || strings.Contains(strings.ToLower(err.Error()), "session not found")
}

// ListTools queries the upstream MCP server for its tool list. If ErrSessionMissing
// occurs, the client automatically re-dials and retries once.
func (c *UpstreamClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		if err := c.dial(ctx); err != nil {
			return nil, fmt.Errorf("upstream not connected: %w", err)
		}
		c.mu.RLock()
		session = c.session
		c.mu.RUnlock()
	}

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil && isSessionMissingError(err) {
		c.logger.Debug("upstream session missing on ListTools, attempting redial", zap.Error(err))
		if redialErr := c.dial(ctx); redialErr == nil {
			c.mu.RLock()
			session = c.session
			c.mu.RUnlock()
			return session.ListTools(ctx, &mcp.ListToolsParams{})
		}
	}
	return res, err
}

// CallTool forwards a tool call to the upstream MCP server. If ErrSessionMissing
// occurs, the client automatically re-dials and retries once.
func (c *UpstreamClient) CallTool(ctx context.Context, name string, args any) (*mcp.CallToolResult, error) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		if err := c.dial(ctx); err != nil {
			return nil, fmt.Errorf("upstream not connected: %w", err)
		}
		c.mu.RLock()
		session = c.session
		c.mu.RUnlock()
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil && isSessionMissingError(err) {
		c.logger.Debug("upstream session missing on CallTool, attempting redial", zap.Error(err))
		if redialErr := c.dial(ctx); redialErr == nil {
			c.mu.RLock()
			session = c.session
			c.mu.RUnlock()
			return session.CallTool(ctx, &mcp.CallToolParams{
				Name:      name,
				Arguments: args,
			})
		}
	}
	return res, err
}

// Close closes the upstream MCP session and terminates background retry routines.
func (c *UpstreamClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

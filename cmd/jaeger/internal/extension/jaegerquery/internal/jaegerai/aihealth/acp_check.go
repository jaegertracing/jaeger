// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aihealth

import (
	"context"
	"encoding/json"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/internal/version"
)

// NewACPCheck returns a check function that opens a fresh WebSocket
// connection to agentURL, performs one ACP `initialize` round-trip, and
// closes. Any transport-level or protocol-level error counts as unhealthy.
// Suitable for use as aihealth.Config.Check.
func NewACPCheck(agentURL string, logger *zap.Logger) func(ctx context.Context) error {
	// The check only sends `initialize` and immediately closes — the sidecar
	// should never send a client-bound call in that window, but if it does we
	// refuse it rather than crash.
	noopMethodHandler := func(_ context.Context, method string, _ json.RawMessage) (any, *acp.RequestError) {
		return nil, acp.NewMethodNotFound(method)
	}
	return func(ctx context.Context) error {
		adapter, err := jaegerai.DialWsAdapter(ctx, agentURL, logger)
		if err != nil {
			return fmt.Errorf("dial: %w", err)
		}
		defer adapter.Close()

		conn := acp.NewConnection(noopMethodHandler, adapter, adapter)

		req := acp.InitializeRequest{
			ProtocolVersion: acp.ProtocolVersionNumber,
			ClientCapabilities: acp.ClientCapabilities{
				Fs:       acp.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
				Terminal: false,
			},
			ClientInfo: &acp.Implementation{
				Name:    "jaeger-ai-check",
				Version: version.Get().GitVersion,
			},
		}
		if _, err := acp.SendRequest[acp.InitializeResponse](conn, ctx, acp.AgentMethodInitialize, req); err != nil {
			return fmt.Errorf("initialize: %w", err)
		}
		return nil
	}
}

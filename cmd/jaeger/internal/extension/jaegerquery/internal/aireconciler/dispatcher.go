// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aireconciler

import (
	"context"
	"encoding/json"

	acp "github.com/coder/acp-go-sdk"
)

// noopMethodHandler returns MethodNotFound for every inbound call. The probe
// only sends an `initialize` request to the sidecar and immediately closes
// the connection — the sidecar should not send any client-bound calls in
// that window, but if it does we refuse them rather than crash.
func noopMethodHandler(_ context.Context, method string, _ json.RawMessage) (any, *acp.RequestError) {
	return nil, acp.NewMethodNotFound(method)
}

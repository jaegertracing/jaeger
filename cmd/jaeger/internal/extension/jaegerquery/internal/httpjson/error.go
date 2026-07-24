// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package httpjson provides JSON response helpers for Jaeger Query HTTP APIs.
package httpjson

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteError writes an error response with a JSON body. It must be called before
// the response is committed.
func WriteError(w http.ResponseWriter, statusCode int, response any) error {
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON error response: %w", err)
	}
	body = append(body, '\n')

	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("failed to write JSON error response: %w", err)
	}
	return nil
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

const routeChat = "/api/ai/chat"

// normalizeBasePath canonicalises the operator-supplied jaeger-query base
// path so route registration agrees on a single prefix. Empty and "/" both
// mean "no prefix" and are returned as "". Otherwise any trailing slash is
// trimmed so concatenating "/api/..." can never produce a double slash.
func normalizeBasePath(basePath string) string {
	if basePath == "" || basePath == "/" {
		return ""
	}
	return strings.TrimSuffix(basePath, "/")
}

// Handler is the entry point for the jaeger-query AI gateway. Callers
// construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all
// AI routing inside the jaegerai package.
type Handler struct {
	logger             *zap.Logger
	agentURL           string
	basePath           string
	maxRequestBodySize int64
}

// NewHandler constructs a jaegerai.Handler. agentURL is the WebSocket
// endpoint of the ACP sidecar; basePath is the jaeger-query base path used
// to prefix the AI routes. maxRequestBodySize bounds the chat request body
// to prevent abuse. basePath is normalized once so the registered mux
// pattern uses a single canonical prefix.
func NewHandler(logger *zap.Logger, agentURL, basePath string, maxRequestBodySize int64) *Handler {
	return &Handler{
		logger:             logger,
		agentURL:           agentURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
	}
}

// RegisterRoutes mounts the AI gateway endpoints on the provided mux. The
// chat endpoint streams ACP turns to/from the sidecar.
func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc(h.basePath+routeChat, NewChatHandler(h.logger, h.agentURL, h.maxRequestBodySize).ServeHTTP)
}

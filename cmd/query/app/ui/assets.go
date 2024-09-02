// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"embed"
	"net/http"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/gzipfs"
	"github.com/jaegertracing/jaeger/pkg/httpfs"
)

//go:embed actual/*
var actualAssetsFS embed.FS

//go:embed placeholder/index.html
var placeholderAssetsFS embed.FS

// GetStaticFiles gets the static assets that the Jaeger UI will serve. If the actual
// assets are available, then this function will return them. Otherwise, a
// non-functional index.html is returned to be used as a placeholder.
func GetStaticFiles(logger *zap.Logger) http.FileSystem {
	if _, err := actualAssetsFS.ReadFile("actual/index.html.gz"); err != nil {
		logger.Warn("ui assets not embedded in the binary, using a placeholder", zap.Error(err))
		return httpfs.PrefixedFS("placeholder", http.FS(placeholderAssetsFS))
	}

	return httpfs.PrefixedFS("actual", http.FS(gzipfs.New(actualAssetsFS)))
}

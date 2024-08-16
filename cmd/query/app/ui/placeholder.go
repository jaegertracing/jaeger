// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !ui
// +build !ui

package ui

import (
	"embed"
	"net/http"

	"github.com/jaegertracing/jaeger/pkg/httpfs"
)

//go:embed placeholder/index.html
var assetsFS embed.FS

// StaticFiles provides http filesystem with static files for UI.
var StaticFiles = httpfs.PrefixedFS("placeholder", http.FS(assetsFS))

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/cmd/query/app/ui"
	"github.com/jaegertracing/jaeger/pkg/fswatcher"
	"github.com/jaegertracing/jaeger/pkg/version"
)

var (
	configPattern      = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern    = regexp.MustCompile(`(?im)^\s*\/\/\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern     = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	compabilityPattern = regexp.MustCompile("JAEGER_STORAGE_CAPABILITIES *= *DEFAULT_STORAGE_CAPABILITIES;")
	basePathPattern    = regexp.MustCompile(`<base href="/"`)
)

// RegisterStaticHandler adds handler for static assets to the router.
func RegisterStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *QueryOptions, qCapabilities querysvc.StorageCapabilities) io.Closer {
	staticHandler, err := NewStaticAssetsHandler(qOpts.UIConfig.AssetsPath, StaticAssetsHandlerOptions{
		UIConfig:            qOpts.UIConfig,
		BasePath:            qOpts.BasePath,
		StorageCapabilities: qCapabilities,
		Logger:              logger,
	})
	if err != nil {
		logger.Panic("Could not create static assets handler", zap.Error(err))
	}

	staticHandler.RegisterRoutes(r)

	return staticHandler
}

type StaticAssetsHandler struct {
	options   StaticAssetsHandlerOptions
	indexHTML atomic.Value
	assetsFS  http.FileSystem
	watcher   *fswatcher.FSWatcher
}

type StaticAssetsHandlerOptions struct {
	UIConfig
	BasePath            string
	StorageCapabilities querysvc.StorageCapabilities
	Logger              *zap.Logger
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, options StaticAssetsHandlerOptions) (*StaticAssetsHandler, error) {
	assetsFS := ui.GetStaticFiles(options.Logger)
	if staticAssetsRoot != "" {
		assetsFS = http.Dir(staticAssetsRoot)
	}

	h := &StaticAssetsHandler{
		options:  options,
		assetsFS: assetsFS,
	}

	indexHTML, err := h.loadAndEnrichIndexHTML(assetsFS.Open)
	if err != nil {
		return nil, err
	}

	options.Logger.Info("Using UI configuration", zap.String("path", options.ConfigFile))
	watcher, err := fswatcher.New([]string{options.ConfigFile}, h.reloadUIConfig, h.options.Logger)
	if err != nil {
		return nil, err
	}
	h.watcher = watcher

	h.indexHTML.Store(indexHTML)

	return h, nil
}

func (sH *StaticAssetsHandler) RegisterRoutes(router *mux.Router) {
	fileServer := http.FileServer(sH.assetsFS)
	fileServerWithCache := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		fileServer.ServeHTTP(w, r)
	})

	router.PathPrefix("/static/").Handler(sH.loggingHandler(fileServerWithCache))
	router.NotFoundHandler = sH.loggingHandler(http.HandlerFunc(sH.notFound))
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.indexHTML.Load().([]byte))
}

func (sH *StaticAssetsHandler) loggingHandler(handler http.Handler) http.Handler {
	if !sH.options.LogAccess {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sH.options.Logger.Info("serving static asset", zap.Stringer("url", r.URL))
		handler.ServeHTTP(w, r)
	})
}

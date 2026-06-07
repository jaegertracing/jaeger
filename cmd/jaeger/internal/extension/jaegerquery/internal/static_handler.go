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

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/ui"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/fswatcher"
	"github.com/jaegertracing/jaeger/internal/version"
)

var (
	// The following patterns are searched and replaced in the index.html as a way of customizing the UI.
	configPattern       = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern     = regexp.MustCompile(`(?im)^\s*//\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern      = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	capabilitiesPattern = regexp.MustCompile("JAEGER_BACKEND_CAPABILITIES *= *DEFAULT_BACKEND_CAPABILITIES;")
)

// BackendCapabilities is the JSON shape injected into index.html via the
// JAEGER_BACKEND_CAPABILITIES search-replace pattern.
type BackendCapabilities struct {
	ArchiveStorage bool `json:"archiveStorage"`
	MetricsStorage bool `json:"metricsStorage"`
	AIAssistant    bool `json:"aiAssistant"`
}

// RegisterStaticHandler builds and registers the static-assets handler on r.
// aiHealthCheck may be nil; the chat surface stays hidden when it is.
// Returns an io.Closer that stops the fswatcher used for UI config reloads.
func RegisterStaticHandler(r *http.ServeMux, logger *zap.Logger, qOpts *QueryOptions, qCapabilities querysvc.StorageCapabilities, aiHealthCheck func() bool) io.Closer {
	h, err := newStaticAssetsHandler(qOpts, qCapabilities, aiHealthCheck, logger)
	if err != nil {
		logger.Panic("Could not create static assets handler", zap.Error(err))
	}
	h.registerRoutes(r)
	return h
}

// staticAssetsHandler serves the Jaeger UI bundle. index.html is derived on
// every SPA serve so all injected values — including the AI capability,
// which flips at runtime — are always current. The raw bytes and the parsed
// UI config are cached; everything else is computed at serve time.
type staticAssetsHandler struct {
	assetsFS      http.FileSystem
	basePath      string
	logAccess     bool
	storageCaps   querysvc.StorageCapabilities
	aiHealthCheck func() bool
	logger        *zap.Logger

	indexHTMLRaw []byte                       // read once from disk at boot
	uiConfig     atomic.Pointer[loadedConfig] // refreshed by fswatcher on UI-config-file changes; nil when no UI config is set
	uiConfigFile string

	watcher *fswatcher.FSWatcher
}

type loadedConfig struct {
	regexp *regexp.Regexp
	config []byte
}

func newStaticAssetsHandler(qOpts *QueryOptions, storageCaps querysvc.StorageCapabilities, aiHealthCheck func() bool, logger *zap.Logger) (*staticAssetsHandler, error) {
	assetsFS := ui.GetStaticFiles(logger)
	if qOpts.UIConfig.AssetsPath != "" {
		assetsFS = http.Dir(qOpts.UIConfig.AssetsPath)
	}
	raw, err := loadIndexHTML(assetsFS.Open)
	if err != nil {
		return nil, fmt.Errorf("cannot load index.html: %w", err)
	}
	h := &staticAssetsHandler{
		assetsFS:      assetsFS,
		basePath:      qOpts.BasePath,
		logAccess:     qOpts.UIConfig.LogAccess,
		storageCaps:   storageCaps,
		aiHealthCheck: aiHealthCheck,
		logger:        logger,
		indexHTMLRaw:  raw,
		uiConfigFile:  qOpts.UIConfig.ConfigFile,
	}
	if err := h.refreshUIConfig(); err != nil {
		return nil, err
	}
	if qOpts.UIConfig.ConfigFile != "" {
		logger.Info("Using UI configuration", zap.String("path", qOpts.UIConfig.ConfigFile))
	}
	watcher, err := fswatcher.New([]string{qOpts.UIConfig.ConfigFile}, h.reloadUIConfig, logger)
	if err != nil {
		return nil, err
	}
	h.watcher = watcher
	return h, nil
}

// refreshUIConfig re-reads the UI config file from disk and atomically swaps
// the cached parsed value. Called once at boot and again whenever the
// fswatcher fires.
func (h *staticAssetsHandler) refreshUIConfig() error {
	cfg, err := loadUIConfig(h.uiConfigFile)
	if err != nil {
		return err
	}
	h.uiConfig.Store(cfg)
	return nil
}

func (h *staticAssetsHandler) reloadUIConfig() {
	h.logger.Info("reloading UI config", zap.String("filename", h.uiConfigFile))
	if err := h.refreshUIConfig(); err != nil {
		h.logger.Error("error while reloading the UI config", zap.Error(err))
		return
	}
	h.logger.Info("reloaded UI config", zap.String("filename", h.uiConfigFile))
}

// deriveIndexHTML builds the served index.html from the cached raw bytes by
// applying every substitution on the spot — UI config, version, backend
// capabilities. Called per request so values that can change at runtime are
// always current.
//
// The <base href> is not injected here. The UI detects its own mount-point
// prefix at page-load time via an inline script in index.html (see ADR-009).
func (h *staticAssetsHandler) deriveIndexHTML() []byte {
	out := h.indexHTMLRaw
	if cfg := h.uiConfig.Load(); cfg != nil {
		out = cfg.regexp.ReplaceAll(out, cfg.config)
	}
	versionJSON, _ := json.Marshal(version.Get())
	out = versionPattern.ReplaceAll(out, fmt.Appendf(nil, "JAEGER_VERSION = %s;", versionJSON))
	aiAvailable := false
	if h.aiHealthCheck != nil {
		aiAvailable = h.aiHealthCheck()
	}
	capsJSON, _ := json.Marshal(BackendCapabilities{
		ArchiveStorage: h.storageCaps.ArchiveStorage,
		MetricsStorage: h.storageCaps.MetricsStorage,
		AIAssistant:    aiAvailable,
	})
	out = capabilitiesPattern.ReplaceAll(out, fmt.Appendf(nil, "JAEGER_BACKEND_CAPABILITIES = %s;", capsJSON))
	return out
}

func loadIndexHTML(open func(string) (http.File, error)) ([]byte, error) {
	indexFile, err := open("/index.html")
	if err != nil {
		return nil, fmt.Errorf("cannot open index.html: %w", err)
	}
	defer indexFile.Close()
	indexBytes, err := io.ReadAll(indexFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read from index.html: %w", err)
	}
	return indexBytes, nil
}

func loadUIConfig(uiConfig string) (*loadedConfig, error) {
	if uiConfig == "" {
		return nil, nil
	}
	bytesConfig, err := os.ReadFile(filepath.Clean(uiConfig))
	if err != nil {
		return nil, fmt.Errorf("cannot read UI config file %v: %w", uiConfig, err)
	}
	var r []byte

	ext := filepath.Ext(uiConfig)
	switch strings.ToLower(ext) {
	case ".json":
		var c map[string]any

		if err := json.Unmarshal(bytesConfig, &c); err != nil {
			return nil, fmt.Errorf("cannot parse UI config file %v: %w", uiConfig, err)
		}
		r, _ = json.Marshal(c)

		return &loadedConfig{
			regexp: configPattern,
			config: append([]byte("JAEGER_CONFIG = "), append(r, byte(';'))...),
		}, nil
	case ".js":
		r = bytes.TrimSpace(bytesConfig)
		re := regexp.MustCompile(`function\s+UIConfig(\s)?\(\s?\)(\s)?{`)
		if !re.Match(r) {
			return nil, fmt.Errorf("UI config file must define function UIConfig(): %v", uiConfig)
		}

		return &loadedConfig{
			regexp: configJsPattern,
			config: r,
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized UI config file format, expecting .js or .json file: %v", uiConfig)
	}
}

func (h *staticAssetsHandler) loggingHandler(handler http.Handler) http.Handler {
	if !h.logAccess {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info("serving static asset", zap.Stringer("url", r.URL))
		handler.ServeHTTP(w, r)
	})
}

func (h *staticAssetsHandler) registerRoutes(router *http.ServeMux) {
	basePath := h.basePath
	if basePath == "" {
		basePath = "/"
	}

	fileServer := http.FileServer(h.assetsFS)
	if basePath != "/" {
		fileServer = http.StripPrefix(basePath+"/", fileServer)
	}

	// Register static files handler
	var staticPattern string
	if basePath == "/" {
		staticPattern = "/static/"
	} else {
		staticPattern = basePath + "/static/"
	}
	router.Handle(staticPattern, h.loggingHandler(fileServer))

	// Register catch-all handler for SPA routing (serves index.html for all non-API routes).
	// This must be registered last to act as a fallback.
	// Note that the invalid /api/* routes return 404 directly.
	var catchAllPattern string
	if basePath == "/" {
		catchAllPattern = "/"
	} else {
		catchAllPattern = basePath + "/"
	}
	router.Handle(catchAllPattern, h.loggingHandler(http.HandlerFunc(h.serveSPA)))
}

func (h *staticAssetsHandler) serveSPA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(h.deriveIndexHTML())
}

func (h *staticAssetsHandler) Close() error {
	return h.watcher.Close()
}

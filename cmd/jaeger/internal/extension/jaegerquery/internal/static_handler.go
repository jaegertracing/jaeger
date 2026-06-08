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
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/ui"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/version"
)

var (
	// The following patterns are searched and replaced in the index.html as a way of customizing the UI.
	configPattern       = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern     = regexp.MustCompile(`(?im)^\s*//\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern      = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	capabilitiesPattern = regexp.MustCompile("JAEGER_BACKEND_CAPABILITIES *= *DEFAULT_BACKEND_CAPABILITIES;")
)

// uiConfigReloadInterval is the TTL on the cached UI config: deriveIndexHTML
// re-reads the file from disk when the cached value is older than this. A
// package-level var (not const) so tests can shorten it. Matches the
// "load from disk on demand, cache for N" pattern used by configtls'
// certificate reloader — no background goroutine needed.
var uiConfigReloadInterval = 10 * time.Second

// BackendCapabilities is the JSON shape injected into index.html via the
// JAEGER_BACKEND_CAPABILITIES search-replace pattern.
type BackendCapabilities struct {
	ArchiveStorage bool `json:"archiveStorage"`
	MetricsStorage bool `json:"metricsStorage"`
	AIAssistant    bool `json:"aiAssistant"`
}

// RegisterStaticHandler builds and registers the static-assets handler on r.
// aiHealthCheck may be nil; the chat surface stays hidden when it is.
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
// which flips at runtime — are always current. The raw bytes are cached;
// the UI config is cached with a TTL and re-read from disk on demand.
type staticAssetsHandler struct {
	assetsFS      http.FileSystem
	basePath      string
	logAccess     bool
	storageCaps   querysvc.StorageCapabilities
	aiHealthCheck func() bool
	logger        *zap.Logger

	indexHTMLRaw []byte // read once from disk at boot
	uiConfigFile string // immutable after construction

	// uiConfig cache. Loaded once at boot for fail-fast validation, then
	// re-read lazily by deriveIndexHTML when uiConfigExpiry has passed.
	uiConfigMu     sync.RWMutex
	uiConfig       *loadedConfig // nil when uiConfigFile == ""
	uiConfigExpiry time.Time
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
	// Eager initial load: surface UI-config syntax errors at startup rather
	// than letting them appear on the first page-load several seconds later.
	cfg, err := loadUIConfig(h.uiConfigFile)
	if err != nil {
		return nil, err
	}
	h.uiConfig = cfg
	h.uiConfigExpiry = time.Now().Add(uiConfigReloadInterval)
	if qOpts.UIConfig.ConfigFile != "" {
		logger.Info("Using UI configuration", zap.String("path", qOpts.UIConfig.ConfigFile))
	}
	return h, nil
}

// getUIConfig returns the cached parsed UI config, re-reading it from disk
// when the TTL has elapsed. A reload error is logged and the previously
// cached value is kept — a transient I/O failure should not break the
// serve path. Returns nil when no UI config file is configured.
func (h *staticAssetsHandler) getUIConfig() *loadedConfig {
	if h.uiConfigFile == "" {
		return nil
	}
	now := time.Now()
	h.uiConfigMu.RLock()
	if now.Before(h.uiConfigExpiry) {
		cfg := h.uiConfig
		h.uiConfigMu.RUnlock()
		return cfg
	}
	h.uiConfigMu.RUnlock()

	h.uiConfigMu.Lock()
	defer h.uiConfigMu.Unlock()
	// Re-check after acquiring the write lock — another goroutine may have
	// refreshed the cache while we were waiting.
	if now.Before(h.uiConfigExpiry) {
		return h.uiConfig
	}
	cfg, err := loadUIConfig(h.uiConfigFile)
	if err != nil {
		h.logger.Error("could not reload UI config; keeping previously cached value",
			zap.String("filename", h.uiConfigFile), zap.Error(err))
	} else {
		h.uiConfig = cfg
	}
	h.uiConfigExpiry = now.Add(uiConfigReloadInterval)
	return h.uiConfig
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
	if cfg := h.getUIConfig(); cfg != nil {
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

// Close is a no-op; the handler holds no resources that need explicit
// teardown. Kept so the type continues to satisfy io.Closer for callers
// that wire it into a chain alongside other closeable handlers.
func (*staticAssetsHandler) Close() error { return nil }

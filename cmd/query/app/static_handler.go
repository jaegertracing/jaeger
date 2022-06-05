// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/ui"
	"github.com/jaegertracing/jaeger/pkg/fswatcher"
	"github.com/jaegertracing/jaeger/pkg/version"
)

var (
	favoriteIcon    = "favicon.ico"
	staticRootFiles = []string{favoriteIcon}

	// The following patterns are searched and replaced in the index.html as a way of customizing the UI.
	configPattern   = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern = regexp.MustCompile(`(?im)^\s*\/\/\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern  = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	basePathPattern = regexp.MustCompile(`<base href="/"`) // Note: tag is not closed
)

// RegisterStaticHandler adds handler for static assets to the router.
func RegisterStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *QueryOptions) {
	staticHandler, err := NewStaticAssetsHandler(qOpts.StaticAssets, StaticAssetsHandlerOptions{
		BasePath:     qOpts.BasePath,
		UIConfigPath: qOpts.UIConfig,
		Logger:       logger,
	})
	if err != nil {
		logger.Panic("Could not create static assets handler", zap.Error(err))
	}

	staticHandler.RegisterRoutes(r)
}

// StaticAssetsHandler handles static assets
type StaticAssetsHandler struct {
	options    StaticAssetsHandlerOptions
	indexHTML  atomic.Value // stores []byte
	assetsFS   http.FileSystem
	newWatcher func() (fswatcher.Watcher, error)
}

// StaticAssetsHandlerOptions defines options for NewStaticAssetsHandler
type StaticAssetsHandlerOptions struct {
	BasePath     string
	UIConfigPath string
	Logger       *zap.Logger
	NewWatcher   func() (fswatcher.Watcher, error)
}

type loadedConfig struct {
	regexp *regexp.Regexp
	config []byte
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, options StaticAssetsHandlerOptions) (*StaticAssetsHandler, error) {
	assetsFS := ui.StaticFiles
	if staticAssetsRoot != "" {
		assetsFS = http.Dir(staticAssetsRoot)
	}

	if options.Logger == nil {
		options.Logger = zap.NewNop()
	}

	if options.NewWatcher == nil {
		options.NewWatcher = fswatcher.NewWatcher
	}

	indexHTML, err := loadAndEnrichIndexHTML(assetsFS.Open, options)
	if err != nil {
		return nil, err
	}

	h := &StaticAssetsHandler{
		options:    options,
		assetsFS:   assetsFS,
		newWatcher: options.NewWatcher,
	}

	h.indexHTML.Store(indexHTML)
	h.watch()

	return h, nil
}

func loadAndEnrichIndexHTML(open func(string) (http.File, error), options StaticAssetsHandlerOptions) ([]byte, error) {
	indexBytes, err := loadIndexHTML(open)
	if err != nil {
		return nil, fmt.Errorf("cannot load index.html: %w", err)
	}
	// replace UI config
	if configObject, err := loadUIConfig(options.UIConfigPath); err != nil {
		return nil, err
	} else if configObject != nil {
		indexBytes = configObject.regexp.ReplaceAll(indexBytes, configObject.config)
	}
	// replace Jaeger version
	versionJSON, _ := json.Marshal(version.Get())
	versionString := fmt.Sprintf("JAEGER_VERSION = %s;", string(versionJSON))
	indexBytes = versionPattern.ReplaceAll(indexBytes, []byte(versionString))
	// replace base path
	if options.BasePath == "" {
		options.BasePath = "/"
	}
	if options.BasePath != "/" {
		if !strings.HasPrefix(options.BasePath, "/") || strings.HasSuffix(options.BasePath, "/") {
			return nil, fmt.Errorf("invalid base path '%s'. Must start but not end with a slash '/', e.g. '/jaeger/ui'", options.BasePath)
		}
		indexBytes = basePathPattern.ReplaceAll(indexBytes, []byte(fmt.Sprintf(`<base href="%s/"`, options.BasePath)))
	}

	return indexBytes, nil
}

func (sH *StaticAssetsHandler) configListener(watcher fswatcher.Watcher) {
	for {
		select {
		case event := <-watcher.Events():
			// ignore if the event filename is not the UI configuration
			if filepath.Base(event.Name) != filepath.Base(sH.options.UIConfigPath) {
				continue
			}
			// ignore if the event is a chmod event (permission or owner changes)
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				sH.options.Logger.Warn("the UI config file has been removed, using the last known version")
				continue
			}
			// this will catch events for all files inside the same directory, which is OK if we don't have many changes
			sH.options.Logger.Info("reloading UI config", zap.String("filename", sH.options.UIConfigPath))
			content, err := loadAndEnrichIndexHTML(sH.assetsFS.Open, sH.options)
			if err != nil {
				sH.options.Logger.Error("error while reloading the UI config", zap.Error(err))
			}
			sH.indexHTML.Store(content)
			sH.options.Logger.Info("reloaded UI config", zap.String("filename", sH.options.UIConfigPath))
		case err, ok := <-watcher.Errors():
			if !ok {
				return
			}
			sH.options.Logger.Error("event", zap.Error(err))
		}
	}
}

func (sH *StaticAssetsHandler) watch() {
	if sH.options.UIConfigPath == "" {
		sH.options.Logger.Info("UI config path not provided, config file will not be watched")
		return
	}

	watcher, err := sH.newWatcher()
	if err != nil {
		sH.options.Logger.Error("failed to create a new watcher for the UI config", zap.Error(err))
		return
	}

	go func() {
		sH.configListener(watcher)
	}()

	if err := watcher.Add(sH.options.UIConfigPath); err != nil {
		sH.options.Logger.Error("error adding watcher to file", zap.String("file", sH.options.UIConfigPath), zap.Error(err))
	} else {
		sH.options.Logger.Info("watching", zap.String("file", sH.options.UIConfigPath))
	}

	dir := filepath.Dir(sH.options.UIConfigPath)
	if err := watcher.Add(dir); err != nil {
		sH.options.Logger.Error("error adding watcher to dir", zap.String("dir", dir), zap.Error(err))
	} else {
		sH.options.Logger.Info("watching", zap.String("dir", dir))
	}
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
		var c map[string]interface{}

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

// RegisterRoutes registers routes for this handler on the given router
func (sH *StaticAssetsHandler) RegisterRoutes(router *mux.Router) {
	fileServer := http.FileServer(sH.assetsFS)
	if sH.options.BasePath != "/" {
		fileServer = http.StripPrefix(sH.options.BasePath+"/", fileServer)
	}
	router.PathPrefix("/static/").Handler(fileServer)
	for _, file := range staticRootFiles {
		router.Path("/" + file).Handler(fileServer)
	}
	router.NotFoundHandler = http.HandlerFunc(sH.notFound)
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.indexHTML.Load().([]byte))
}

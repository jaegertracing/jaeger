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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/ui"
)

var (
	favoriteIcon    = "favicon.ico"
	staticRootFiles = []string{favoriteIcon}
	configPattern   = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	basePathPattern = regexp.MustCompile(`<base href="/"`)
	basePathReplace = `<base href="%s/"`
	errBadBasePath  = "Invalid base path '%s'. Must start but not end with a slash '/', e.g. '/jaeger/ui'"
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
	options   StaticAssetsHandlerOptions
	indexHTML atomic.Value // stores []byte
	assetsFS  http.FileSystem
}

// StaticAssetsHandlerOptions defines options for NewStaticAssetsHandler
type StaticAssetsHandlerOptions struct {
	BasePath     string
	UIConfigPath string
	Logger       *zap.Logger
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

	indexHTML, err := loadIndexBytes(assetsFS.Open, options)
	if err != nil {
		return nil, err
	}

	h := &StaticAssetsHandler{
		options:  options,
		assetsFS: assetsFS,
	}

	h.indexHTML.Store(indexHTML)
	h.watch()

	return h, nil
}

func loadIndexBytes(open func(string) (http.File, error), options StaticAssetsHandlerOptions) ([]byte, error) {
	indexBytes, err := loadIndexHTML(open)
	if err != nil {
		return nil, errors.Wrap(err, "cannot load index.html")
	}
	configString := "JAEGER_CONFIG = DEFAULT_CONFIG"
	if config, err := loadUIConfig(options.UIConfigPath); err != nil {
		return nil, err
	} else if config != nil {
		// TODO if we want to support other config formats like YAML, we need to normalize `config` to be
		// suitable for json.Marshal(). For example, YAML parser may return a map that has keys of type
		// interface{}, and json.Marshal() is unable to serialize it.
		bytes, _ := json.Marshal(config)
		configString = fmt.Sprintf("JAEGER_CONFIG = %v", string(bytes))
	}
	indexBytes = configPattern.ReplaceAll(indexBytes, []byte(configString+";"))
	if options.BasePath == "" {
		options.BasePath = "/"
	}
	if options.BasePath != "/" {
		if !strings.HasPrefix(options.BasePath, "/") || strings.HasSuffix(options.BasePath, "/") {
			return nil, fmt.Errorf(errBadBasePath, options.BasePath)
		}
		indexBytes = basePathPattern.ReplaceAll(indexBytes, []byte(fmt.Sprintf(basePathReplace, options.BasePath)))
	}

	return indexBytes, nil
}

func (sH *StaticAssetsHandler) watch() {
	if sH.options.UIConfigPath == "" {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		sH.options.Logger.Error("failed to create a new watcher for the UI config", zap.Error(err))
		return
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					// this might be related to a file inside the dir, so, just log a warn if this is about the file we care about
					// otherwise, just ignore the event
					if event.Name == sH.options.UIConfigPath {
						sH.options.Logger.Warn("the UI config file has been removed, using the last known version")
					}
					continue
				}

				// this will catch events for all files inside the same directory, which is OK if we don't have many changes
				sH.options.Logger.Info("reloading UI config", zap.String("filename", sH.options.UIConfigPath))

				content, err := loadIndexBytes(sH.assetsFS.Open, sH.options)
				if err != nil {
					sH.options.Logger.Error("error while reloading the UI config", zap.Error(err))
				}

				sH.indexHTML.Store(content)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				sH.options.Logger.Error("event", zap.Error(err))
			}
		}
	}()

	err = watcher.Add(sH.options.UIConfigPath)
	if err != nil {
		sH.options.Logger.Error("error adding watcher to file", zap.String("file", sH.options.UIConfigPath), zap.Error(err))
	} else {
		sH.options.Logger.Info("watching", zap.String("file", sH.options.UIConfigPath))
	}

	dir := filepath.Dir(sH.options.UIConfigPath)
	err = watcher.Add(dir)
	if err != nil {
		sH.options.Logger.Error("error adding watcher to dir", zap.String("dir", dir), zap.Error(err))
	} else {
		sH.options.Logger.Info("watching", zap.String("dir", dir))
	}
}

func loadIndexHTML(open func(string) (http.File, error)) ([]byte, error) {
	indexFile, err := open("/index.html")
	if err != nil {
		return nil, errors.Wrap(err, "cannot open index.html")
	}
	defer indexFile.Close()
	indexBytes, err := ioutil.ReadAll(indexFile)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read from index.html")
	}
	return indexBytes, nil
}

func loadUIConfig(uiConfig string) (map[string]interface{}, error) {
	if uiConfig == "" {
		return nil, nil
	}
	ext := filepath.Ext(uiConfig)
	bytes, err := ioutil.ReadFile(uiConfig) /* nolint #nosec , this comes from an admin, not user */
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read UI config file %v", uiConfig)
	}

	var c map[string]interface{}
	var unmarshal func([]byte, interface{}) error

	switch strings.ToLower(ext) {
	case ".json":
		unmarshal = json.Unmarshal
	default:
		return nil, fmt.Errorf("unrecognized UI config file format %v", uiConfig)
	}

	if err := unmarshal(bytes, &c); err != nil {
		return nil, errors.Wrapf(err, "cannot parse UI config file %v", uiConfig)
	}
	return c, nil
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

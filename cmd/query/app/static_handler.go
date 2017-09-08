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
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

const (
	defaultStaticAssetsRoot = "jaeger-ui-build/build/"
)

var (
	staticRootFiles = []string{"favicon.ico"}
)

// StaticAssetsHandler handles static assets
type StaticAssetsHandler struct {
	staticAssetsRoot string
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string) *StaticAssetsHandler {
	if staticAssetsRoot == "" {
		staticAssetsRoot = defaultStaticAssetsRoot
	}
	if !strings.HasSuffix(staticAssetsRoot, "/") {
		staticAssetsRoot = staticAssetsRoot + "/"
	}
	return &StaticAssetsHandler{staticAssetsRoot: staticAssetsRoot}
}

// RegisterRoutes registers routes for this handler on the given router
func (sH *StaticAssetsHandler) RegisterRoutes(router *mux.Router) {
	router.PathPrefix("/static").Handler(http.FileServer(http.Dir(sH.staticAssetsRoot)))
	for _, file := range staticRootFiles {
		router.Path("/" + file).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, sH.staticAssetsRoot+file)
		})
	}
	router.NotFoundHandler = http.HandlerFunc(sH.notFound)
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, r *http.Request) {
	// don't allow returning "304 Not Modified" for index.html because
	// the cached versions might have the wrong filenames for javascript assets
	delete(r.Header, "If-Modified-Since")
	http.ServeFile(w, r, sH.staticAssetsRoot+"index.html")
}

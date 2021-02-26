// Copyright (c) 2021 The Jaeger Authors.
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

package httpfs

import (
	"net/http"
)

// PrefixedFS returns a FileSystem that adds a path prefix to all files
// before delegating to the underlying fs.
func PrefixedFS(prefix string, fs http.FileSystem) http.FileSystem {
	return &prefixedFS{
		prefix: prefix,
		fs:     fs,
	}
}

type prefixedFS struct {
	prefix string
	fs     http.FileSystem
}

func (fs *prefixedFS) Open(name string) (http.File, error) {
	prefixedName := fs.prefix + name
	if name == "/" {
		// Return the dir itself when asked for the root.
		// This is what http.FS() also does to allow redirects
		// from `/`` to `/index.html`.
		prefixedName = fs.prefix
	}
	return fs.fs.Open(prefixedName)
}

// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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

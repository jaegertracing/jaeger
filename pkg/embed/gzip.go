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

package embed

import (
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"time"
)

const suffix = ".gz"

type File struct {
	file    fs.File
	content []byte
	offset  int
}

type FileInfo struct {
	info fs.FileInfo
	size int64
}

type FileSystem struct {
	embed embed.FS
}

func (f File) Stat() (fs.FileInfo, error) {
	stat, err := f.file.Stat()
	if err != nil {
		return stat, err
	}
	return FileInfo{
		info: stat,
		size: int64(len(f.content)),
	}, nil
}

func (f *File) Read(buf []byte) (n int, err error) {
	if len(buf) > len(f.content)-f.offset {
		buf = buf[0:len(f.content[f.offset:])]
	}

	n = copy(buf, f.content[f.offset:])
	if n == len(f.content)-f.offset {
		return n, io.EOF
	}
	f.offset += n
	return
}

func (f File) Close() error {
	return f.file.Close()
}

func (fi FileInfo) Name() string {
	name := fi.info.Name()
	return name[:len(name)-len(suffix)]
}

func (fi FileInfo) Size() int64 { return fi.size }

func (fi FileInfo) Mode() fs.FileMode { return fi.info.Mode() }

func (fi FileInfo) ModTime() time.Time { return fi.info.ModTime() }

func (fi FileInfo) IsDir() bool { return fi.info.IsDir() }

func (fi FileInfo) Sys() interface{} { return nil }

func New(fs embed.FS) FileSystem {
	return FileSystem{fs}
}

func (cfs FileSystem) Open(path string) (fs.File, error) {
	var f fs.File
	f, err := cfs.embed.Open(path)
	if err == nil {
		return f, nil
	}

	f, err = cfs.embed.Open(path + suffix)
	if err != nil {
		return f, err
	}

	gr, err := gzip.NewReader(f)
	if err != nil {
		return f, err
	}
	defer gr.Close()

	c, err := io.ReadAll(gr)
	if err != nil {
		return f, err
	}

	return &File{
		file:    f,
		content: c,
	}, nil
}

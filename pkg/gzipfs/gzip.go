// Copyright (c) 2021 The Jaeger Authors.
// Copyright 2021 The Prometheus Authors.
// SPDX-License-Identifier: Apache-2.0

package gzipfs

import (
	"compress/gzip"
	"io"
	"io/fs"
	"time"
)

const suffix = ".gz"

type file struct {
	file    fs.File
	content []byte
	offset  int
}

type fileInfo struct {
	info fs.FileInfo
	size int64
}

type fileSystem struct {
	fs fs.FS
}

func (f file) Stat() (fs.FileInfo, error) {
	stat, err := f.file.Stat()
	if err != nil {
		return nil, err
	}
	return fileInfo{
		info: stat,
		size: int64(len(f.content)),
	}, nil
}

func (f *file) Read(buf []byte) (int, error) {
	if len(buf) > len(f.content)-f.offset {
		buf = buf[0:len(f.content[f.offset:])]
	}

	n := copy(buf, f.content[f.offset:])
	if n == len(f.content)-f.offset {
		return n, io.EOF
	}
	f.offset += n
	return n, nil
}

func (f file) Close() error {
	return f.file.Close()
}

func (fi fileInfo) Name() string {
	name := fi.info.Name()
	return name[:len(name)-len(suffix)]
}

func (fi fileInfo) Size() int64 { return fi.size }

func (fi fileInfo) Mode() fs.FileMode { return fi.info.Mode() }

func (fi fileInfo) ModTime() time.Time { return fi.info.ModTime() }

func (fi fileInfo) IsDir() bool { return fi.info.IsDir() }

func (fileInfo) Sys() any { return nil }

// New wraps underlying fs that is expected to contain gzipped files
// and presents an unzipped view of it.
func New(filesystem fs.FS) fs.FS {
	return fileSystem{filesystem}
}

func (cfs fileSystem) Open(path string) (fs.File, error) {
	var f fs.File
	f, err := cfs.fs.Open(path)
	if err == nil {
		return f, nil
	}

	f, err = cfs.fs.Open(path + suffix)
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

	return &file{
		file:    f,
		content: c,
	}, nil
}

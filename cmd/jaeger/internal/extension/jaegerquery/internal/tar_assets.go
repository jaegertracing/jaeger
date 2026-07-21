// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

type tarGzipFS struct {
	entries map[string]*tarGzipEntry
}

type tarGzipEntry struct {
	name     string
	mode     fs.FileMode
	modTime  time.Time
	data     []byte
	children []*tarGzipEntry
}

type tarGzipFile struct {
	entry  *tarGzipEntry
	reader *bytes.Reader
	offset int
}

func assetsFileSystem(assetsPath string) (http.FileSystem, error) {
	if !strings.HasSuffix(assetsPath, ".tar.gz") {
		return http.Dir(assetsPath), nil
	}
	info, err := os.Stat(filepath.Clean(assetsPath))
	if err != nil {
		return nil, fmt.Errorf("cannot inspect UI assets path %q: %w", assetsPath, err)
	}
	if !info.Mode().IsRegular() {
		return http.Dir(assetsPath), nil
	}

	archiveFS, err := newTarGzipFS(assetsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load UI assets archive %q: %w", assetsPath, err)
	}
	return http.FS(archiveFS), nil
}

func newTarGzipFS(filename string) (fs.FS, error) {
	file, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return readTarGzipFS(file)
}

func readTarGzipFS(reader io.Reader) (fs.FS, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("cannot read gzip stream: %w", err)
	}
	defer gzipReader.Close()

	entries := map[string]*tarGzipEntry{
		".": {
			name: ".",
			mode: fs.ModeDir | 0o555,
		},
	}
	var indexPaths []string
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read tar stream: %w", err)
		}

		name := strings.TrimSuffix(strings.TrimPrefix(header.Name, "./"), "/")
		if name == "" && header.Typeflag == tar.TypeDir {
			continue
		}
		if !fs.ValidPath(name) || path.Clean(name) != name {
			return nil, fmt.Errorf("invalid archive path %q", header.Name)
		}
		if _, ok := entries[name]; ok {
			return nil, fmt.Errorf("duplicate archive path %q", header.Name)
		}

		entry := &tarGzipEntry{
			name:    name,
			mode:    header.FileInfo().Mode(),
			modTime: header.ModTime,
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			entry.data, err = io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("cannot read archive file %q: %w", header.Name, err)
			}
			if path.Base(name) == "index.html" {
				indexPaths = append(indexPaths, name)
			}
		default:
			return nil, fmt.Errorf("unsupported archive entry %q with type %d", header.Name, header.Typeflag)
		}
		entries[name] = entry
	}

	root, err := archiveRoot(indexPaths)
	if err != nil {
		return nil, err
	}
	if err := addImplicitDirectories(entries); err != nil {
		return nil, err
	}
	archiveFS := &tarGzipFS{entries: entries}
	if root == "." {
		return archiveFS, nil
	}
	return fs.Sub(archiveFS, root)
}

func archiveRoot(indexPaths []string) (string, error) {
	if len(indexPaths) == 0 {
		return "", errors.New("archive does not contain index.html")
	}
	for _, indexPath := range indexPaths {
		if indexPath == "index.html" {
			return ".", nil
		}
	}
	if len(indexPaths) > 1 {
		return "", errors.New("archive contains multiple index.html files")
	}
	return path.Dir(indexPaths[0]), nil
}

func addImplicitDirectories(entries map[string]*tarGzipEntry) error {
	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if entry, ok := entries[parent]; ok {
				if !entry.IsDir() {
					return fmt.Errorf("archive path %q has non-directory parent %q", name, parent)
				}
				continue
			}
			entries[parent] = &tarGzipEntry{
				name: parent,
				mode: fs.ModeDir | 0o555,
			}
		}
	}

	for name, entry := range entries {
		if name == "." {
			continue
		}
		parent := entries[path.Dir(name)]
		if !parent.IsDir() {
			return fmt.Errorf("archive path %q has non-directory parent %q", name, parent.name)
		}
		parent.children = append(parent.children, entry)
	}
	for _, entry := range entries {
		slices.SortFunc(entry.children, func(a, b *tarGzipEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
	}
	return nil
}

func (f *tarGzipFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := f.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &tarGzipFile{
		entry:  entry,
		reader: bytes.NewReader(entry.data),
	}, nil
}

func (*tarGzipFile) Close() error { return nil }

func (f *tarGzipFile) Read(data []byte) (int, error) {
	if f.entry.IsDir() {
		return 0, &fs.PathError{Op: "read", Path: f.entry.name, Err: syscall.EISDIR}
	}
	return f.reader.Read(data)
}

func (f *tarGzipFile) Seek(offset int64, whence int) (int64, error) {
	if f.entry.IsDir() {
		return 0, &fs.PathError{Op: "seek", Path: f.entry.name, Err: syscall.EISDIR}
	}
	return f.reader.Seek(offset, whence)
}

func (f *tarGzipFile) Stat() (fs.FileInfo, error) { return f.entry, nil }

func (f *tarGzipFile) ReadDir(count int) ([]fs.DirEntry, error) {
	if !f.entry.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: f.entry.name, Err: syscall.ENOTDIR}
	}
	if count <= 0 {
		children := make([]fs.DirEntry, len(f.entry.children)-f.offset)
		for i, child := range f.entry.children[f.offset:] {
			children[i] = child
		}
		f.offset = len(f.entry.children)
		return children, nil
	}
	if f.offset >= len(f.entry.children) {
		return nil, io.EOF
	}
	end := min(f.offset+count, len(f.entry.children))
	children := make([]fs.DirEntry, end-f.offset)
	for i, child := range f.entry.children[f.offset:end] {
		children[i] = child
	}
	f.offset = end
	return children, nil
}

func (e *tarGzipEntry) Name() string {
	return path.Base(e.name)
}

func (e *tarGzipEntry) Size() int64 { return int64(len(e.data)) }

func (e *tarGzipEntry) Mode() fs.FileMode { return e.mode }

func (e *tarGzipEntry) ModTime() time.Time { return e.modTime }

func (e *tarGzipEntry) IsDir() bool { return e.mode.IsDir() }

func (*tarGzipEntry) Sys() any { return nil }

func (e *tarGzipEntry) Type() fs.FileMode { return e.mode.Type() }

func (e *tarGzipEntry) Info() (fs.FileInfo, error) { return e, nil }

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
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// tarGzSuffix marks an assets path as an archive rather than a directory.
// It matches the name of the assets bundle published on the jaeger-ui
// releases page, so a distribution can point ui.assets_path straight at the
// downloaded file without unpacking it first.
const tarGzSuffix = ".tar.gz"

// tarGzFS is a read-only fs.FS over a gzipped tar archive.
//
// The archive is expanded into memory once at construction rather than being
// decompressed per request: tar is a sequential format with no index, so
// serving a single asset from disk would mean re-scanning the archive from the
// start every time. The UI bundle is a few megabytes, which is the same order
// as the assets already embedded in the binary by ui.GetStaticFiles.
type tarGzFS struct {
	// entries is keyed by fs.FS path — slash-separated, unrooted, with the
	// archive root stored as ".".
	entries map[string]*tarGzEntry
}

type tarGzEntry struct {
	name     string // base name, as reported by FileInfo.Name
	data     []byte // nil for directories
	mode     fs.FileMode
	modTime  time.Time
	isDir    bool
	children []*tarGzEntry // populated for directories only
}

// newTarGzFS reads the archive at archivePath into memory.
func newTarGzFS(archivePath string) (fs.FS, error) {
	f, err := os.Open(filepath.Clean(archivePath))
	if err != nil {
		return nil, fmt.Errorf("cannot open assets archive %s: %w", archivePath, err)
	}
	defer f.Close()
	return readTarGz(f, archivePath)
}

// readTarGz builds the in-memory tree from a gzipped tar stream. name is used
// only to give errors a useful subject.
func readTarGz(r io.Reader, name string) (fs.FS, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read assets archive %s: %w", name, err)
	}
	defer gr.Close()

	fsys := &tarGzFS{entries: make(map[string]*tarGzEntry)}
	fsys.entries["."] = &tarGzEntry{
		name:  ".",
		mode:  fs.ModeDir | 0o555,
		isDir: true,
	}

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read assets archive %s: %w", name, err)
		}

		entryPath := path.Clean(strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./"))
		// Skip the root itself and any entry whose name escapes the archive
		// root (e.g. "../evil"). A read-only FS has nothing to gain from
		// honouring traversal, and fs.FS forbids such paths outright.
		if entryPath == "." || !fs.ValidPath(entryPath) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			fsys.mkdirAll(entryPath, hdr.ModTime)
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("cannot read %s from assets archive %s: %w", entryPath, name, err)
			}
			fsys.addFile(entryPath, data, hdr)
		default:
			// Symlinks, devices and the like have no meaning for the UI
			// bundle; ignore them rather than failing the whole archive.
		}
	}

	// Directory listings must be ordered by filename to satisfy fs.ReadDirFile.
	for _, e := range fsys.entries {
		if e.isDir {
			sort.Slice(e.children, func(i, j int) bool {
				return e.children[i].name < e.children[j].name
			})
		}
	}
	return fsys, nil
}

// addFile inserts a regular file, creating any missing parent directories.
func (fsys *tarGzFS) addFile(entryPath string, data []byte, hdr *tar.Header) {
	if existing, ok := fsys.entries[entryPath]; ok && !existing.isDir {
		// Later entries win, mirroring how an extraction to disk would end up.
		existing.data = data
		existing.modTime = hdr.ModTime
		return
	}
	parent := fsys.mkdirAll(path.Dir(entryPath), hdr.ModTime)
	e := &tarGzEntry{
		name:    path.Base(entryPath),
		data:    data,
		mode:    fs.FileMode(hdr.Mode).Perm(),
		modTime: hdr.ModTime,
	}
	fsys.entries[entryPath] = e
	parent.children = append(parent.children, e)
}

// mkdirAll returns the directory entry for dir, creating it and any missing
// ancestors. Archives are not required to contain explicit directory entries.
func (fsys *tarGzFS) mkdirAll(dir string, modTime time.Time) *tarGzEntry {
	if dir == "." || dir == "" {
		return fsys.entries["."]
	}
	if e, ok := fsys.entries[dir]; ok && e.isDir {
		return e
	}
	parent := fsys.mkdirAll(path.Dir(dir), modTime)
	e := &tarGzEntry{
		name:    path.Base(dir),
		mode:    fs.ModeDir | 0o555,
		modTime: modTime,
		isDir:   true,
	}
	fsys.entries[dir] = e
	parent.children = append(parent.children, e)
	return e
}

func (fsys *tarGzFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	e, ok := fsys.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if e.isDir {
		return &tarGzDir{entry: e}, nil
	}
	return &tarGzFile{Reader: bytes.NewReader(e.data), entry: e}, nil
}

// tarGzFile embeds *bytes.Reader so the file satisfies io.Seeker, which
// http.ServeContent needs for range requests.
type tarGzFile struct {
	*bytes.Reader
	entry *tarGzEntry
}

func (f *tarGzFile) Stat() (fs.FileInfo, error) { return tarGzInfo{f.entry}, nil }

func (*tarGzFile) Close() error { return nil }

type tarGzDir struct {
	entry  *tarGzEntry
	offset int
}

func (d *tarGzDir) Stat() (fs.FileInfo, error) { return tarGzInfo{d.entry}, nil }

func (*tarGzDir) Close() error { return nil }

func (d *tarGzDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.entry.name, Err: fs.ErrInvalid}
}

func (d *tarGzDir) ReadDir(n int) ([]fs.DirEntry, error) {
	remaining := d.entry.children[d.offset:]
	if n <= 0 {
		d.offset = len(d.entry.children)
		return toDirEntries(remaining), nil
	}
	if len(remaining) == 0 {
		return nil, io.EOF
	}
	if n > len(remaining) {
		n = len(remaining)
	}
	d.offset += n
	return toDirEntries(remaining[:n]), nil
}

func toDirEntries(entries []*tarGzEntry) []fs.DirEntry {
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, fs.FileInfoToDirEntry(tarGzInfo{e}))
	}
	return out
}

type tarGzInfo struct{ entry *tarGzEntry }

func (i tarGzInfo) Name() string       { return i.entry.name }
func (i tarGzInfo) Size() int64        { return int64(len(i.entry.data)) }
func (i tarGzInfo) Mode() fs.FileMode  { return i.entry.mode }
func (i tarGzInfo) ModTime() time.Time { return i.entry.modTime }
func (i tarGzInfo) IsDir() bool        { return i.entry.isDir }
func (tarGzInfo) Sys() any             { return nil }

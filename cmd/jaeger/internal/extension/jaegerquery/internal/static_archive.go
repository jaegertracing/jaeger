// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"go.uber.org/zap"
)

const (
	// uiArchiveSuffix marks an assets_path that points at a UI release archive
	// rather than a directory.
	uiArchiveSuffix = ".tar.gz"

	// uiArchivePrefix is the directory the jaeger-ui release packs its build
	// under in assets.tar.gz. scripts/build/rebuild-ui.sh extracts the same
	// prefix, and only entries below it are served.
	uiArchivePrefix = "packages/jaeger-ui/build/"
)

// openAssetsFS returns the file system the UI is served from. A regular file
// whose name ends in .tar.gz is loaded as a jaeger-ui release archive; any
// other path is served as a directory, as before.
func openAssetsFS(assetsPath string, logger *zap.Logger) (http.FileSystem, error) {
	if !strings.HasSuffix(assetsPath, uiArchiveSuffix) {
		return http.Dir(assetsPath), nil
	}
	info, err := os.Stat(assetsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open UI assets archive %s: %w", assetsPath, err)
	}
	if !info.Mode().IsRegular() {
		return http.Dir(assetsPath), nil
	}
	return loadAssetsArchive(assetsPath, logger)
}

// loadAssetsArchive reads the regular files of a jaeger-ui assets.tar.gz that
// sit under uiArchivePrefix into an in-memory file system, with the prefix
// stripped so index.html is at the root. The whole build is decompressed into
// memory once, at startup. Entries outside the prefix, entries whose names are
// not clean relative paths, and anything other than regular files are
// skipped. Each file keeps its modification time from the archive, so
// Last-Modified does not change across restarts.
func loadAssetsArchive(archivePath string, logger *zap.Logger) (http.FileSystem, error) {
	f, err := os.Open(filepath.Clean(archivePath))
	if err != nil {
		return nil, fmt.Errorf("cannot open UI assets archive %s: %w", archivePath, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("cannot read UI assets archive %s: %w", archivePath, err)
	}
	defer gzr.Close()

	memFS := afero.NewMemMapFs()
	files := 0
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read UI assets archive %s: %w", archivePath, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := strings.CutPrefix(hdr.Name, uiArchivePrefix)
		// fs.ValidPath rejects names that are not clean slash-separated
		// paths; filepath.IsLocal is the check CodeQL recognizes against
		// path traversal ("Zip Slip").
		if !ok || !fs.ValidPath(name) || !filepath.IsLocal(name) {
			continue
		}
		// WriteReader creates any missing parent directories.
		err = afero.WriteReader(memFS, "/"+name, tr)
		if err == nil {
			err = memFS.Chtimes("/"+name, hdr.ModTime, hdr.ModTime)
		}
		if err != nil {
			return nil, fmt.Errorf("cannot load %s from UI assets archive %s: %w", hdr.Name, archivePath, err)
		}
		files++
	}
	// The tar reader stops at the end-of-archive marker, which can leave
	// padding and the gzip trailer unread. Drain the stream so a truncated or
	// corrupt gzip stream fails here instead of being served. A tar cut short
	// at an entry boundary before compression is not detected.
	if err := drain(gzr); err != nil {
		return nil, fmt.Errorf("cannot read UI assets archive %s: %w", archivePath, err)
	}
	if files == 0 {
		return nil, fmt.Errorf("UI assets archive %s has no files under %s", archivePath, uiArchivePrefix)
	}
	logger.Info("Serving UI assets from archive", zap.String("path", archivePath), zap.Int("files", files))
	return afero.NewHttpFs(memFS), nil
}

// drain reads r to the end and discards what it reads.
func drain(r io.Reader) error {
	buf := make([]byte, 32<<10)
	for {
		_, err := r.Read(buf)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

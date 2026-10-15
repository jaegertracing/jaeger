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

// maxUIArchiveSize caps how many bytes loadAssetsArchive decompresses, so a
// malformed or hostile archive cannot exhaust memory. A jaeger-ui build is a
// few megabytes. A package-level var (not const) so tests can lower it.
var maxUIArchiveSize int64 = 256 << 20

// openAssetsFS returns the file system the UI is served from. A regular file
// whose name ends in .tar.gz is loaded as a jaeger-ui release archive; any
// other path is served as a directory, as before.
func openAssetsFS(assetsPath string) (http.FileSystem, error) {
	info, err := os.Stat(assetsPath)
	if err == nil && !info.IsDir() && strings.HasSuffix(assetsPath, uiArchiveSuffix) {
		return loadAssetsArchive(assetsPath)
	}
	return http.Dir(assetsPath), nil
}

// loadAssetsArchive reads the entries of a jaeger-ui assets.tar.gz that sit
// under uiArchivePrefix into an in-memory file system, with the prefix
// stripped so index.html is at the root. Entries outside the prefix, entries
// whose names are not clean relative paths, and anything other than regular
// files and directories are skipped.
func loadAssetsArchive(archivePath string) (http.FileSystem, error) {
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

	src := &io.LimitedReader{R: gzr, N: maxUIArchiveSize + 1}
	// fail reports the size cap in place of err when the cap is what cut the
	// stream short, since the tar reader only sees an early EOF.
	fail := func(err error) error {
		if src.N <= 0 {
			return fmt.Errorf("UI assets archive %s exceeds %d bytes uncompressed", archivePath, maxUIArchiveSize)
		}
		return err
	}

	memFS := afero.NewMemMapFs()
	files := 0
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail(fmt.Errorf("cannot read UI assets archive %s: %w", archivePath, err))
		}
		name, ok := strings.CutPrefix(hdr.Name, uiArchivePrefix)
		if !ok {
			continue
		}
		name = strings.TrimSuffix(name, "/")
		if name == "" || !fs.ValidPath(name) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = memFS.MkdirAll("/"+name, 0o755)
		case tar.TypeReg:
			err = afero.WriteReader(memFS, "/"+name, tr)
			files++
		default:
			continue
		}
		if err != nil {
			return nil, fail(fmt.Errorf("cannot load %s from UI assets archive %s: %w", hdr.Name, archivePath, err))
		}
	}
	// The tar reader stops at the end-of-archive marker, which can leave
	// padding and the gzip trailer unread. Drain the stream so a truncated or
	// corrupt archive fails its checksum here instead of being served.
	if _, err := io.Copy(io.Discard, src); err != nil {
		return nil, fmt.Errorf("cannot read UI assets archive %s: %w", archivePath, err)
	}
	if err := fail(nil); err != nil {
		return nil, err
	}
	if files == 0 {
		return nil, fmt.Errorf("UI assets archive %s has no files under %s", archivePath, uiArchivePrefix)
	}
	return afero.NewHttpFs(memFS), nil
}

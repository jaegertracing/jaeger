// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// buildFixtureArchive packs the existing "fixture" directory into a
// .tar.gz in a temp dir, so the archive and the directory cases are served
// from byte-identical sources.
func buildFixtureArchive(t *testing.T) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "fixture.tar.gz")
	out, err := os.Create(archivePath)
	require.NoError(t, err)
	defer out.Close()

	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)

	const root = "fixture"
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return archivePath
}

func serveFromAssets(t *testing.T, assetsPath, target string) (int, string) {
	t.Helper()
	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: assetsPath}},
		querysvc.StorageCapabilities{},
		nil,
	)
	defer closer.Close()

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, http.NoBody))
	return rr.Code, rr.Body.String()
}

// TestAssetsFromArchiveMatchDirectory is the core guarantee: pointing
// assets_path at the archive serves exactly what the directory serves.
func TestAssetsFromArchiveMatchDirectory(t *testing.T) {
	archive := buildFixtureArchive(t)

	for _, target := range []string{"/", "/static/asset.txt"} {
		t.Run(target, func(t *testing.T) {
			dirCode, dirBody := serveFromAssets(t, "fixture", target)
			arcCode, arcBody := serveFromAssets(t, archive, target)

			assert.Equal(t, http.StatusOK, dirCode)
			assert.Equal(t, dirCode, arcCode)
			assert.Equal(t, dirBody, arcBody)
		})
	}
}

func TestNewAssetsFSFallsBackToDir(t *testing.T) {
	// A directory that merely ends in .tar.gz is still a directory.
	dir := filepath.Join(t.TempDir(), "assets.tar.gz")
	require.NoError(t, os.Mkdir(dir, 0o755))

	tests := []struct {
		name string
		path string
	}{
		{"plain directory", "fixture"},
		{"directory named like an archive", dir},
		{"archive that does not exist", filepath.Join(t.TempDir(), "missing.tar.gz")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assetsFS, err := newAssetsFS(tt.path)
			require.NoError(t, err)
			assert.IsType(t, http.Dir(""), assetsFS)
		})
	}
}

func TestNewAssetsFSRejectsMalformedArchive(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "broken.tar.gz")
	require.NoError(t, os.WriteFile(bad, []byte("not gzip at all"), 0o600))

	_, err := newAssetsFS(bad)
	require.ErrorContains(t, err, "cannot read assets archive")
}

func TestNewStaticAssetsHandlerMalformedArchive(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "broken.tar.gz")
	require.NoError(t, os.WriteFile(bad, []byte("not gzip at all"), 0o600))

	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: bad}},
		querysvc.StorageCapabilities{},
		nil,
		zap.NewNop(),
	)
	require.ErrorContains(t, err, "cannot read assets archive")
	assert.Nil(t, handler)
}

func TestNewTarGzFSUnreadableArchive(t *testing.T) {
	_, err := newTarGzFS(filepath.Join(t.TempDir(), "nope.tar.gz"))
	require.ErrorContains(t, err, "cannot open assets archive")
}

func TestTarGzFSTruncatedArchive(t *testing.T) {
	// Valid gzip wrapper, truncated tar payload.
	full, err := os.ReadFile(buildFixtureArchive(t))
	require.NoError(t, err)

	truncated := filepath.Join(t.TempDir(), "truncated.tar.gz")
	require.NoError(t, os.WriteFile(truncated, full[:len(full)/2], 0o600))

	_, err = newTarGzFS(truncated)
	require.Error(t, err)
}

func TestTarGzFSTruncatedFileContent(t *testing.T) {
	const content = "some index.html content"

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "index.html", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content)),
	}))
	_, err := tw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	// Keep the 512-byte header block but cut the payload short, so gzip and the
	// tar header both parse and the failure surfaces while reading the entry.
	partial := tarBuf.Bytes()[:512+len(content)/2]

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err = gw.Write(partial)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	_, err = readTarGz(bytes.NewReader(gzBuf.Bytes()), "partial.tar.gz")
	require.ErrorContains(t, err, "cannot read index.html from assets archive")
}

// TestTarGzFSCompliance leans on the stdlib's own fs.FS conformance suite,
// which exercises Open, Stat, ReadDir and path validation far more thoroughly
// than hand-written cases would.
func TestTarGzFSCompliance(t *testing.T) {
	f, err := os.Open(buildFixtureArchive(t))
	require.NoError(t, err)
	defer f.Close()

	fsys, err := readTarGz(f, "fixture.tar.gz")
	require.NoError(t, err)
	require.NoError(t, fstest.TestFS(fsys, "index.html", "static/asset.txt"))
}

func TestTarGzFSOpenErrors(t *testing.T) {
	f, err := os.Open(buildFixtureArchive(t))
	require.NoError(t, err)
	defer f.Close()

	fsys, err := readTarGz(f, "fixture.tar.gz")
	require.NoError(t, err)

	t.Run("invalid path", func(t *testing.T) {
		_, err := fsys.Open("../escape")
		require.ErrorIs(t, err, fs.ErrInvalid)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := fsys.Open("does/not/exist")
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("read on a directory", func(t *testing.T) {
		d, err := fsys.Open("static")
		require.NoError(t, err)
		defer d.Close()

		_, err = d.Read(make([]byte, 1))
		require.ErrorIs(t, err, fs.ErrInvalid)
	})
}

func TestTarGzFSDirectoryListing(t *testing.T) {
	f, err := os.Open(buildFixtureArchive(t))
	require.NoError(t, err)
	defer f.Close()

	fsys, err := readTarGz(f, "fixture.tar.gz")
	require.NoError(t, err)

	d, err := fsys.Open("static")
	require.NoError(t, err)
	defer d.Close()

	info, err := d.Stat()
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, "static", info.Name())
	assert.Nil(t, info.Sys())

	dir, ok := d.(fs.ReadDirFile)
	require.True(t, ok, "directories must satisfy fs.ReadDirFile")

	// Paged reads drain the listing and then report io.EOF.
	first, err := dir.ReadDir(1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "asset.txt", first[0].Name())

	_, err = dir.ReadDir(1)
	require.ErrorIs(t, err, io.EOF)
}

func TestTarGzFSSkipsUnsupportedEntries(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "odd.tar.gz")
	out, err := os.Create(archivePath)
	require.NoError(t, err)

	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)

	// A symlink and a traversal attempt are both ignored; the regular file
	// alongside them is still served. The nested file also proves that parent
	// directories are synthesised when the archive omits them.
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "link", Typeflag: tar.TypeSymlink, Linkname: "index.html", Mode: 0o777,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3,
	}))
	_, err = tw.Write([]byte("bad"))
	require.NoError(t, err)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "./nested/deep/ok.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2,
	}))
	_, err = tw.Write([]byte("ok"))
	require.NoError(t, err)

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, out.Close())

	fsys, err := newTarGzFS(archivePath)
	require.NoError(t, err)

	data, err := fs.ReadFile(fsys, "nested/deep/ok.txt")
	require.NoError(t, err)
	assert.Equal(t, "ok", string(data))

	_, err = fsys.Open("link")
	require.ErrorIs(t, err, fs.ErrNotExist)

	_, err = fsys.Open("escape.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestTarGzFSDuplicateEntriesLastWins(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "dupes.tar.gz")
	out, err := os.Create(archivePath)
	require.NoError(t, err)

	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)

	// tar allows the same path to appear twice; extracting to disk would leave
	// the later copy in place, so the FS view must agree.
	for _, content := range []string{"first", "last!"} {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: "index.html", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content)),
		}))
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, out.Close())

	fsys, err := newTarGzFS(archivePath)
	require.NoError(t, err)

	data, err := fs.ReadFile(fsys, "index.html")
	require.NoError(t, err)
	assert.Equal(t, "last!", string(data))

	// The duplicate must not be listed twice in its parent directory.
	entries, err := fs.ReadDir(fsys, ".")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "index.html", entries[0].Name())
}

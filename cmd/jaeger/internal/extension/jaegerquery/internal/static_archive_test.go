// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type archiveEntry struct {
	name     string
	typeflag byte
	body     string
}

func buildTar(t *testing.T, entries []archiveEntry) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o644}
		switch e.typeflag {
		case tar.TypeDir:
			hdr.Mode = 0o755
		case tar.TypeSymlink:
			hdr.Linkname = e.body
		default:
			hdr.Size = int64(len(e.body))
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if hdr.Size > 0 {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func gzipBytes(t *testing.T, data []byte) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	_, err := gzw.Write(data)
	require.NoError(t, err)
	require.NoError(t, gzw.Close())
	return buf.Bytes()
}

func writeArchive(t *testing.T, data []byte) string {
	p := filepath.Join(t.TempDir(), "assets.tar.gz")
	require.NoError(t, os.WriteFile(p, data, 0o600))
	return p
}

// releaseEntries mirrors the layout of the assets.tar.gz published with a
// jaeger-ui release, where the build sits under packages/jaeger-ui/build/.
func releaseEntries() []archiveEntry {
	return []archiveEntry{
		{name: uiArchivePrefix, typeflag: tar.TypeDir},
		{name: uiArchivePrefix + "index.html", typeflag: tar.TypeReg, body: "<html>archived JAEGER_VERSION = DEFAULT_VERSION;</html>"},
		{name: uiArchivePrefix + "static/", typeflag: tar.TypeDir},
		{name: uiArchivePrefix + "static/app.js", typeflag: tar.TypeReg, body: "console.log('archived');"},
	}
}

func readFromFS(t *testing.T, fsys http.FileSystem, name string) string {
	f, err := fsys.Open(name)
	require.NoError(t, err)
	defer f.Close()
	b, err := io.ReadAll(f)
	require.NoError(t, err)
	return string(b)
}

func TestStaticHandlerServesAssetsArchive(t *testing.T) {
	archive := writeArchive(t, gzipBytes(t, buildTar(t, releaseEntries())))
	h, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: archive}},
		nilBackendCaps,
		zap.NewNop(),
	)
	require.NoError(t, err)
	r := http.NewServeMux()
	h.registerRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	for _, tc := range []struct {
		path     string
		contains string
	}{
		{path: "/", contains: "archived JAEGER_VERSION = {"},
		{path: "/search", contains: "archived JAEGER_VERSION = {"},
		{path: "/static/app.js", contains: "console.log('archived');"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), tc.contains)
		})
	}
}

func TestLoadAssetsArchiveSkipsEntries(t *testing.T) {
	entries := append(releaseEntries(),
		archiveEntry{name: "README.md", typeflag: tar.TypeReg, body: "outside the prefix"},
		archiveEntry{name: "packages/jaeger-ui/index.html", typeflag: tar.TypeReg, body: "outside the prefix"},
		archiveEntry{name: uiArchivePrefix + "../escape.txt", typeflag: tar.TypeReg, body: "escapes the prefix"},
		archiveEntry{name: uiArchivePrefix + "static/../../escape.txt", typeflag: tar.TypeReg, body: "escapes the prefix"},
		archiveEntry{name: uiArchivePrefix + "/absolute.txt", typeflag: tar.TypeReg, body: "absolute"},
		archiveEntry{name: uiArchivePrefix + "link.txt", typeflag: tar.TypeSymlink, body: "/etc/passwd"},
	)
	fsys, err := loadAssetsArchive(writeArchive(t, gzipBytes(t, buildTar(t, entries))))
	require.NoError(t, err)

	assert.Equal(t, "<html>archived JAEGER_VERSION = DEFAULT_VERSION;</html>", readFromFS(t, fsys, "/index.html"))
	assert.Equal(t, "console.log('archived');", readFromFS(t, fsys, "/static/app.js"))
	for _, name := range []string{
		"/README.md",
		"/packages/jaeger-ui/index.html",
		"/escape.txt",
		"/static/escape.txt",
		"/absolute.txt",
		"/link.txt",
	} {
		_, err := fsys.Open(name)
		require.Error(t, err, name)
	}
}

func TestLoadAssetsArchiveWithoutDirectoryEntries(t *testing.T) {
	entries := []archiveEntry{
		{name: uiArchivePrefix + "index.html", typeflag: tar.TypeReg, body: "index"},
		{name: uiArchivePrefix + "static/nested/app.js", typeflag: tar.TypeReg, body: "app"},
	}
	fsys, err := loadAssetsArchive(writeArchive(t, gzipBytes(t, buildTar(t, entries))))
	require.NoError(t, err)
	assert.Equal(t, "index", readFromFS(t, fsys, "/index.html"))
	assert.Equal(t, "app", readFromFS(t, fsys, "/static/nested/app.js"))
}

func TestLoadAssetsArchiveErrors(t *testing.T) {
	release := gzipBytes(t, buildTar(t, releaseEntries()))

	// Corrupt the CRC-32 in the gzip trailer. The tar reader reaches the
	// end-of-archive marker before the trailer, so only draining the stream
	// detects it.
	badChecksum := bytes.Clone(release)
	badChecksum[len(badChecksum)-8] ^= 0xff

	tarball := buildTar(t, releaseEntries())
	// Cut the archive inside the body of the last file entry.
	truncatedEntry := tarball[:bytes.Index(tarball, []byte("console.log"))+4]

	for _, tc := range []struct {
		name     string
		data     []byte
		expected string
	}{
		{name: "not gzip", data: []byte("not a gzip stream"), expected: "cannot read UI assets archive"},
		{name: "not tar", data: gzipBytes(t, []byte("not a tar stream")), expected: "cannot read UI assets archive"},
		{name: "bad gzip checksum", data: badChecksum, expected: "gzip: invalid checksum"},
		{name: "truncated entry", data: gzipBytes(t, truncatedEntry), expected: "cannot load " + uiArchivePrefix + "static/app.js"},
		{
			name: "flat archive",
			data: gzipBytes(t, buildTar(t, []archiveEntry{
				{name: "index.html", typeflag: tar.TypeReg, body: "index"},
			})),
			expected: "has no files under " + uiArchivePrefix,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadAssetsArchive(writeArchive(t, tc.data))
			require.ErrorContains(t, err, tc.expected)
		})
	}

	t.Run("too large", func(t *testing.T) {
		// GNU tar pads the archive to a whole record after the end-of-archive
		// marker; the tar reader never reads that padding, only the drain does.
		padded := append(bytes.Clone(tarball), make([]byte, 8192)...)
		for _, tc := range []struct {
			name  string
			data  []byte
			limit int64
		}{
			{name: "inside the first header", data: release, limit: 10},
			{name: "inside the trailing padding", data: gzipBytes(t, padded), limit: int64(len(tarball)) + 10},
		} {
			t.Run(tc.name, func(t *testing.T) {
				archive := writeArchive(t, tc.data)
				saved := maxUIArchiveSize
				maxUIArchiveSize = tc.limit
				t.Cleanup(func() { maxUIArchiveSize = saved })
				_, err := loadAssetsArchive(archive)
				require.ErrorContains(t, err, "exceeds")
			})
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := loadAssetsArchive(filepath.Join(t.TempDir(), "missing.tar.gz"))
		require.ErrorContains(t, err, "cannot open UI assets archive")
	})

	t.Run("through the handler", func(t *testing.T) {
		_, err := newStaticAssetsHandler(
			&QueryOptions{UIConfig: UIConfig{AssetsPath: writeArchive(t, []byte("not a gzip stream"))}},
			nilBackendCaps,
			zap.NewNop(),
		)
		require.ErrorContains(t, err, "cannot read UI assets archive")
	})
}

func TestOpenAssetsFSFallsBackToDirectory(t *testing.T) {
	dir := t.TempDir()
	notArchive := filepath.Join(dir, "assets.zip")
	require.NoError(t, os.WriteFile(notArchive, []byte("zip"), 0o600))
	archiveDir := filepath.Join(dir, "ui.tar.gz")
	require.NoError(t, os.Mkdir(archiveDir, 0o755))

	for _, p := range []string{
		dir,
		notArchive,
		archiveDir,
		filepath.Join(dir, "missing.tar.gz"),
	} {
		fsys, err := openAssetsFS(p)
		require.NoError(t, err)
		assert.Equal(t, http.Dir(p), fsys, p)
	}
}

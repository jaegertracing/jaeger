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
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

type tarTestEntry struct {
	name     string
	contents string
	typeflag byte
}

func TestStaticAssetsFromTarGzip(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "assets.tar.gz")
	writeTarGzip(t, archivePath, []tarTestEntry{
		{name: "packages/jaeger-ui/build/index.html", contents: "<html>archive index</html>", typeflag: tar.TypeReg},
		{name: "packages/jaeger-ui/build/static/asset.txt", contents: "archive asset", typeflag: tar.TypeReg},
	})

	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: archivePath}},
		querysvc.StorageCapabilities{},
		nil,
		zap.NewNop(),
	)
	require.NoError(t, err)
	defer handler.Close()

	mux := http.NewServeMux()
	handler.registerRoutes(mux)
	for _, testCase := range []struct {
		path     string
		expected string
	}{
		{path: "/", expected: "archive index"},
		{path: "/static/asset.txt", expected: "archive asset"},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, http.NoBody))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), testCase.expected)
	}
}

func TestReadTarGzipFS(t *testing.T) {
	archive := tarGzipBytes(t, []tarTestEntry{
		{name: "./", typeflag: tar.TypeDir},
		{name: "index.html", contents: "index", typeflag: tar.TypeReg},
		{name: "static/b.txt", contents: "b", typeflag: tar.TypeReg},
		{name: "static/a.txt", contents: "a", typeflag: tar.TypeReg},
	})
	archiveFS, err := readTarGzipFS(bytes.NewReader(archive))
	require.NoError(t, err)
	require.NoError(t, fstest.TestFS(archiveFS, "index.html", "static/a.txt", "static/b.txt"))

	contents, err := fs.ReadFile(archiveFS, "static/a.txt")
	require.NoError(t, err)
	assert.Equal(t, "a", string(contents))

	root, err := archiveFS.Open(".")
	require.NoError(t, err)
	defer root.Close()
	_, err = root.Read(make([]byte, 1))
	require.ErrorIs(t, err, syscall.EISDIR)
	directory := root.(fs.ReadDirFile)
	entries, err := directory.ReadDir(1)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "index.html", entries[0].Name())
	entries, err = directory.ReadDir(10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "static", entries[0].Name())
	_, err = directory.ReadDir(1)
	require.ErrorIs(t, err, io.EOF)

	file, err := archiveFS.Open("index.html")
	require.NoError(t, err)
	_, err = file.(fs.ReadDirFile).ReadDir(1)
	require.Error(t, err)
	assert.NoError(t, file.Close())
}

func TestReadTarGzipFSErrors(t *testing.T) {
	testCases := []struct {
		name     string
		archive  []byte
		expected string
	}{
		{
			name:     "invalid gzip",
			archive:  []byte("not gzip"),
			expected: "cannot read gzip stream",
		},
		{
			name: "missing index",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "static/asset.txt", contents: "asset", typeflag: tar.TypeReg},
			}),
			expected: "archive does not contain index.html",
		},
		{
			name: "multiple roots",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "one/index.html", contents: "one", typeflag: tar.TypeReg},
				{name: "two/index.html", contents: "two", typeflag: tar.TypeReg},
			}),
			expected: "archive contains multiple index.html files",
		},
		{
			name: "unsafe path",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "../index.html", contents: "index", typeflag: tar.TypeReg},
			}),
			expected: "invalid archive path",
		},
		{
			name: "path traversal within archive root",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "static/../index.html", contents: "index", typeflag: tar.TypeReg},
			}),
			expected: "invalid archive path",
		},
		{
			name: "duplicate path",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "index.html", contents: "one", typeflag: tar.TypeReg},
				{name: "index.html", contents: "two", typeflag: tar.TypeReg},
			}),
			expected: "duplicate archive path",
		},
		{
			name: "unsupported entry",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "index.html", contents: "index", typeflag: tar.TypeReg},
				{name: "link", typeflag: tar.TypeSymlink},
			}),
			expected: "unsupported archive entry",
		},
		{
			name: "non-directory parent",
			archive: tarGzipBytes(t, []tarTestEntry{
				{name: "index.html", contents: "index", typeflag: tar.TypeReg},
				{name: "static", contents: "not a directory", typeflag: tar.TypeReg},
				{name: "static/asset.txt", contents: "asset", typeflag: tar.TypeReg},
			}),
			expected: "has non-directory parent",
		},
		{
			name:     "truncated file",
			archive:  truncatedTarGzipBytes(t),
			expected: "cannot read archive file",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := readTarGzipFS(bytes.NewReader(testCase.archive))
			require.ErrorContains(t, err, testCase.expected)
		})
	}
}

func TestAssetsFileSystemFallsBackToDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "assets.tar.gz")
	require.NoError(t, os.Mkdir(directory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "index.html"), []byte("directory index"), 0o600))

	assetsFS, err := assetsFileSystem(directory)
	require.NoError(t, err)
	index, err := assetsFS.Open("/index.html")
	require.NoError(t, err)
	defer index.Close()
	contents, err := io.ReadAll(index)
	require.NoError(t, err)
	assert.Equal(t, "directory index", string(contents))

	assetsFS, err = assetsFileSystem(filepath.Join(t.TempDir(), "missing.tar.gz"))
	require.Error(t, err)
	assert.Nil(t, assetsFS)
}

func TestNewTarGzipFSOpenError(t *testing.T) {
	_, err := newTarGzipFS(filepath.Join(t.TempDir(), "missing.tar.gz"))
	require.Error(t, err)
}

func writeTarGzip(t *testing.T, filename string, entries []tarTestEntry) {
	t.Helper()
	require.NoError(t, os.WriteFile(filename, tarGzipBytes(t, entries), 0o600))
}

func tarGzipBytes(t *testing.T, entries []tarTestEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     0o644,
			Size:     int64(len(entry.contents)),
			Typeflag: entry.typeflag,
		}
		if entry.typeflag == tar.TypeDir {
			header.Mode = 0o755
			header.Size = 0
		}
		require.NoError(t, tarWriter.WriteHeader(header))
		if header.Size > 0 {
			_, err := tarWriter.Write([]byte(entry.contents))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return output.Bytes()
}

func truncatedTarGzipBytes(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name:     "index.html",
		Mode:     0o644,
		Size:     10,
		Typeflag: tar.TypeReg,
	}))
	_, err := tarWriter.Write([]byte("short"))
	require.NoError(t, err)
	require.Error(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return output.Bytes()
}

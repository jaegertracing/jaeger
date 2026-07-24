// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

func TestStaticAssetsFromTarGzip(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "assets.tar.gz")
	writeTarGzip(t, archivePath, "fixture")

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
		{path: "/", expected: "JAEGER_CONFIG=DEFAULT_CONFIG"},
		{path: "/static/asset.txt", expected: "asset"},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, http.NoBody))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), testCase.expected)
	}
}

func writeTarGzip(t *testing.T, archivePath, root string) {
	t.Helper()
	archive, err := os.Create(filepath.Clean(archivePath))
	require.NoError(t, err)
	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == root {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(header.Name)
		if err := tarWriter.WriteHeader(header); err != nil || !info.Mode().IsRegular() {
			return err
		}
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}))
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, archive.Close())
}

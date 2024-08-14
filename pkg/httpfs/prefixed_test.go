// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package httpfs

import (
	"embed"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

//go:embed test_assets/*
var assetFS embed.FS

func TestPrefixedFS(t *testing.T) {
	fs := PrefixedFS("test_assets", http.FS(assetFS))
	tests := []struct {
		file  string
		isDir bool
	}{
		{file: "/", isDir: true},
		{file: "/somefile.txt", isDir: false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			file, err := fs.Open(tt.file)
			require.NoError(t, err)
			require.NotNil(t, file)
			stat, err := file.Stat()
			require.NoError(t, err)
			require.Equal(t, tt.isDir, stat.IsDir())
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

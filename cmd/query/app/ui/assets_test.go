// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"embed"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/query/app/ui/testdata"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestGetStaticFiles_ReturnsPlaceholderWhenActualNotPresent(t *testing.T) {
	// swap out the assets FS for an empty one and then replace it back when the
	// test completes
	currentActualAssets := actualAssetsFS
	actualAssetsFS = embed.FS{}
	t.Cleanup(func() { actualAssetsFS = currentActualAssets })

	logger, logBuf := testutils.NewLogger()
	fs := GetStaticFiles(logger)
	file, err := fs.Open("/index.html")
	require.NoError(t, err)
	bytes, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Contains(t, string(bytes), "This is a placeholder for the Jaeger UI home page")
	require.Contains(t, logBuf.String(), "ui assets not embedded in the binary, using a placeholder")
}

func TestGetStaticFiles_ReturnsActualWhenPresent(t *testing.T) {
	// swap out the assets FS for a dummy one and then replace it back when the
	// test completes
	currentActualAssets := actualAssetsFS
	actualAssetsFS = testdata.TestFS
	t.Cleanup(func() { actualAssetsFS = currentActualAssets })

	logger, logBuf := testutils.NewLogger()
	fs := GetStaticFiles(logger)
	file, err := fs.Open("/index.html")
	require.NoError(t, err)
	bytes, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NotContains(t, string(bytes), "This is a placeholder for the Jaeger UI home page")
	require.NotContains(t, logBuf.String(), "ui assets not embedded in the binary, using a placeholder")
}

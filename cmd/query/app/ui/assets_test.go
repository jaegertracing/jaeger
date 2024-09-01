// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"embed"
	"io"
	"strings"
	"testing"

	"github.com/crossdock/crossdock-go/require"
	"go.uber.org/zap"
)

func TestGetStaticFiles_ReturnsPlaceholderWhenActualNotPresent(t *testing.T) {
	// swap out the assets FS for an empty one and then replace it back when the
	// test completes
	currentActualAssets := actualAssetsFS
	actualAssetsFS = embed.FS{}
	t.Cleanup(func() { actualAssetsFS = currentActualAssets })

	fs := GetStaticFiles(zap.NewNop())
	file, err := fs.Open("/index.html")
	require.NoError(t, err)
	buf := new(strings.Builder)
	_, err = io.Copy(buf, file)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "This is a placeholder for the Jaeger UI home page")
}

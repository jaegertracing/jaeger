// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httpfs

import (
	"embed"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed test_assets/*
var assetFS embed.FS

// httpfs.AddPrefixFS("web_assets", http.FS(assetFS)),

func TestAddPrefixFS_Root(t *testing.T) {
	fs := AddPrefixFS("test_assets", http.FS(assetFS))
	root, err := fs.Open("/")
	require.NoError(t, err)
	require.NotNil(t, root)
	stat, err := root.Stat()
	require.NoError(t, err)
	require.True(t, stat.IsDir())
}

func TestAddPrefixFS_File(t *testing.T) {
	fs := AddPrefixFS("test_assets", http.FS(assetFS))
	root, err := fs.Open("/somefile.txt")
	require.NoError(t, err)
	require.NotNil(t, root)
	stat, err := root.Stat()
	require.NoError(t, err)
	require.False(t, stat.IsDir())
}

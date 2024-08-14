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

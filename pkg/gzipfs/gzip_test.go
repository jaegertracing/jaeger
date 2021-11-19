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

package gzipfs_test

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/pkg/gzipfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata
var EmbedFS embed.FS

var testFS = gzipfs.New(EmbedFS)

func TestFS(t *testing.T) {
	cases := []struct {
		name            string
		path            string
		expectedName    string
		expectedMode    fs.FileMode
		expectedSize    int64
		expectedContent string
		expectedModTime time.Time
	}{
		{
			name:            "uncompressed file",
			path:            "testdata/foobar",
			expectedMode:    0444,
			expectedName:    "foobar",
			expectedSize:    11,
			expectedContent: "hello world",
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:            "compressed file",
			path:            "testdata/foobar.gz",
			expectedMode:    0444,
			expectedName:    "foobar.gz",
			expectedSize:    38,
			expectedContent: "", // actual gzipped payload is returned
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:            "compressed file accessed without gz extension",
			path:            "testdata/foobaz",
			expectedMode:    0444,
			expectedName:    "foobaz",
			expectedSize:    11,
			expectedContent: "hello world",
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := testFS.Open(c.path)
			require.NoError(t, err)
			defer f.Close()

			stat, err := f.Stat()
			require.NoError(t, err)

			assert.Equal(t, c.expectedName, stat.Name())
			assert.Equal(t, c.expectedMode, stat.Mode())
			assert.Equal(t, c.expectedSize, stat.Size())
			assert.Equal(t, c.expectedModTime, stat.ModTime())
			assert.False(t, stat.IsDir())
			assert.Nil(t, stat.Sys())
			content, err := ioutil.ReadAll(f)
			require.NoError(t, err)
			if c.expectedContent != "" {
				assert.Equal(t, c.expectedContent, string(content))
			}
		})
	}
}

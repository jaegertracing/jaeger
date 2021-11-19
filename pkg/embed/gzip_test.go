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

package embed

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

//go:embed testdata
var EmbedFS embed.FS

var testFS = New(EmbedFS)

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
			expectedContent: "hello world",
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:            "compressed file without gz extension",
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
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}

			name := stat.Name()
			if name != c.expectedName {
				t.Fatalf("name is wrong, expected %s, got %s", c.expectedName, name)
			}

			mode := stat.Mode()
			if mode != c.expectedMode {
				t.Fatalf("mode is wrong, expected %d, got %d", c.expectedMode, mode)
			}

			size := stat.Size()
			if size != c.expectedSize {
				t.Fatalf("size is wrong, expected %d, got %d", c.expectedSize, size)
			}

			modtime := stat.ModTime()
			if modtime != c.expectedModTime {
				t.Fatalf("modtime is wrong, expected %s, got %s", c.expectedModTime, modtime)
			}

			if stat.IsDir() {
				t.Fatalf("isdir is wrong, expected %t, got %t", false, true)
			}

			if stat.Sys() != nil {
				t.Fatalf("stat is wrong, expected nil got non-nil")
			}

			if strings.HasSuffix(c.path, ".gz") {
				return
			}

			content, err := ioutil.ReadAll(f)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != c.expectedContent {
				t.Fatalf("content is wrong, expected %s, got %s", c.expectedContent, string(content))
			}
		})
	}
}

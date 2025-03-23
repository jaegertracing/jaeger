// Copyright (c) 2021 The Jaeger Authors.
// Copyright 2021 The Prometheus Authors.
// SPDX-License-Identifier: Apache-2.0

package gzipfs

import (
	"embed"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

//go:embed testdata
var EmbedFS embed.FS

var testFS = New(EmbedFS)

type mockFile struct {
	err error
}

func (f *mockFile) Stat() (fs.FileInfo, error) {
	return nil, f.err
}

func (f *mockFile) Read([]byte) (int, error) {
	return 0, f.err
}

func (f *mockFile) Close() error {
	return f.err
}

func TestFS(t *testing.T) {
	cases := []struct {
		name            string
		path            string
		expectedErr     string
		expectedName    string
		expectedMode    fs.FileMode
		expectedSize    int64
		expectedContent string
		expectedModTime time.Time
	}{
		{
			name:            "uncompressed file",
			path:            "testdata/foobar",
			expectedMode:    0o444,
			expectedName:    "foobar",
			expectedSize:    11,
			expectedContent: "hello world",
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:            "compressed file",
			path:            "testdata/foobar.gz",
			expectedMode:    0o444,
			expectedName:    "foobar.gz",
			expectedSize:    38,
			expectedContent: "", // actual gzipped payload is returned
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:            "compressed file accessed without gz extension",
			path:            "testdata/foobaz",
			expectedMode:    0o444,
			expectedName:    "foobaz",
			expectedSize:    11,
			expectedContent: "hello world",
			expectedModTime: time.Date(1, 1, 1, 0, 0, 0, 0 /* nanos */, time.UTC),
		},
		{
			name:        "non-existing file",
			path:        "testdata/non-existing-file",
			expectedErr: "file does not exist",
		},
		{
			name:        "not gzipped file",
			path:        "testdata/not_archive",
			expectedErr: "invalid header",
		},
		{
			// To provide coverage of the error from io.ReadAll function, we use a file
			// that is a copy of proper gzipped file testdata/foobaz.gz but truncated
			// to 36 bytes with:
			//     perl -e "truncate 'internal/gzipfs/testdata/foobaz_truncated.gz', 36"
			// This allows gzip.NewReader() to succeed because the file has a proper gz
			// header, but subsequent read fails with unexpected EOF.
			name:        "compressed but truncated file accessed without gz extension",
			path:        "testdata/foobaz_truncated",
			expectedErr: "unexpected EOF",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := testFS.Open(c.path)
			if c.expectedErr != "" {
				require.ErrorContains(t, err, c.expectedErr)
				return
			}
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
			content, err := io.ReadAll(f)
			require.NoError(t, err)
			if c.expectedContent != "" {
				assert.Equal(t, c.expectedContent, string(content))
			}
		})
	}
}

func TestFileStatError(t *testing.T) {
	f := &file{file: &mockFile{assert.AnError}}
	_, err := f.Stat()
	assert.Equal(t, assert.AnError, err)
}

func TestFileRead(t *testing.T) {
	f := &file{content: ([]byte)("long content")}
	buf := make([]byte, 5) // shorter buffer
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

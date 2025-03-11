// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestReadBuf_EOF(t *testing.T) {
	b := ReadBuf{}
	n, err := b.Read(nil)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestReadBuf_Read(t *testing.T) {
	b := &ReadBuf{bytes: []byte("hello"), n: 5}
	r := make([]byte, 5)
	n, err := b.Read(r)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(r))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

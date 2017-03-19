// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package servers

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(r))
}

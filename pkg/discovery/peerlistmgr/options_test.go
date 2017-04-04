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

package peerlistmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestOptions(t *testing.T) {
	logger := zap.NewNop()
	minPeers := 42
	connCheck := 42 * time.Millisecond
	connTimeout := 42 * time.Millisecond
	opts := Options.apply(
		Options.Logger(logger),
		Options.MinPeers(minPeers),
		Options.ConnCheckFrequency(connCheck),
		Options.ConnCheckTimeout(connTimeout),
	)
	assert.Equal(t, logger, opts.logger)
	assert.Equal(t, minPeers, opts.minPeers)
	assert.Equal(t, connCheck, opts.connCheckFrequency)
	assert.Equal(t, connTimeout, opts.connCheckTimeout)
}

func TestOptionsDefaults(t *testing.T) {
	opts := Options.apply()
	assert.NotNil(t, opts.logger)
	assert.Equal(t, defaultMinPeers, opts.minPeers)
	assert.Equal(t, defaultConnCheckFrequency, opts.connCheckFrequency)
	assert.Equal(t, defaultConnCheckTimeout, opts.connCheckTimeout)
}

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
	"time"

	"github.com/uber-go/zap"
)

const (
	defaultMinPeers           = 3
	defaultConnCheckFrequency = time.Second
	defaultConnCheckTimeout   = 250 * time.Millisecond
)

type options struct {
	minPeers           int
	logger             zap.Logger
	connCheckFrequency time.Duration
	connCheckTimeout   time.Duration
}

// Option is a function that sets some option.
type Option func(*options)

// Options is a factory for different options.
var Options = options{}

// Logger creates an Option that assigns the logger.
func (o options) Logger(logger zap.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// MinPeers changes min number of open connections to peers that the manager will try to maintain.
func (o options) MinPeers(minPeers int) Option {
	return func(o *options) {
		o.minPeers = minPeers
	}
}

// ConnCheckFrequency changes how frequently manager will check for MinPeers connections.
func (o options) ConnCheckFrequency(connCheckFrequency time.Duration) Option {
	return func(o *options) {
		o.connCheckFrequency = connCheckFrequency
	}
}

// ConnCheckTimeout changes the timeout used when establishing new connections.
func (o options) ConnCheckTimeout(connCheckTimeout time.Duration) Option {
	return func(o *options) {
		o.connCheckTimeout = connCheckTimeout
	}
}

func (o options) apply(opts ...Option) options {
	ret := options{}
	for _, opt := range opts {
		opt(&ret)
	}
	if ret.logger == nil {
		ret.logger = zap.New(zap.NullEncoder())
	}
	if ret.minPeers <= 0 {
		ret.minPeers = defaultMinPeers
	}
	if ret.connCheckFrequency <= 0 {
		ret.connCheckFrequency = defaultConnCheckFrequency
	}
	if ret.connCheckTimeout <= 0 {
		ret.connCheckTimeout = defaultConnCheckTimeout
	}
	return ret
}

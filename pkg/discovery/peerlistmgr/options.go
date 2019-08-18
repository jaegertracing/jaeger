// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package peerlistmgr

import (
	"time"

	"go.uber.org/zap"
)

const (
	defaultMinPeers           = 3
	defaultConnCheckFrequency = time.Second
	defaultConnCheckTimeout   = 250 * time.Millisecond
)

type options struct {
	minPeers           int
	logger             *zap.Logger
	connCheckFrequency time.Duration
	connCheckTimeout   time.Duration
}

// Option is a function that sets some option.
type Option func(*options)

// Options is a factory for different options.
var Options = options{}

// Logger creates an Option that assigns the logger.
func (o options) Logger(logger *zap.Logger) Option {
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
		ret.logger = zap.NewNop()
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

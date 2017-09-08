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

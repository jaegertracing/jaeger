// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	opts := NewOptions()
	primary := opts.GetConfig()
	assert.NotEmpty(t, primary.Schema.Keyspace)
	assert.NotEmpty(t, primary.Connection.Servers)
	assert.Equal(t, 2, primary.Connection.ConnectionsPerHost)
}

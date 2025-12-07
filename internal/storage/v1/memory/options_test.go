// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := Options{
		Configuration: Configuration{
			MaxTraces: 100,
		},
	}

	assert.Equal(t, 100, opts.Configuration.MaxTraces)
}

// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSamplingCache(t *testing.T) {
	var (
		c         = SamplingCache{}
		service   = "svc"
		operation = "op"
	)
	c.Set(service, operation, &SamplingCacheEntry{})
	assert.NotNil(t, c.Get(service, operation))
	assert.Nil(t, c.Get("blah", "blah"))
}

// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependencyLinkApplyDefaults(t *testing.T) {
	dl := DependencyLink{}.ApplyDefaults()
	assert.Equal(t, JaegerDependencyLinkSource, dl.Source)

	networkSource := "network"
	dl = DependencyLink{Source: networkSource}.ApplyDefaults()
	assert.Equal(t, networkSource, dl.Source)
}

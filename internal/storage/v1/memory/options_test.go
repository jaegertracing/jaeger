// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{"--memory.max-traces=100"})
	opts := Options{}
	opts.InitFromViper(v)

	assert.Equal(t, 100, opts.Configuration.MaxTraces)
}

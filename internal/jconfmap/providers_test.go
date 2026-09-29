// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jconfmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
)

func TestResolverSettings(t *testing.T) {
	set := ResolverSettings()
	var schemes []string
	for _, factory := range set.ProviderFactories {
		schemes = append(schemes, factory.Create(confmap.ProviderSettings{}).Scheme())
	}
	assert.Equal(t, []string{"env", "file", "http", "https", "yaml"}, schemes)
	assert.Equal(t, "env", set.DefaultScheme, "${VAR} without a scheme reads the environment")
}

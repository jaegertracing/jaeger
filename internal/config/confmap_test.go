// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
)

func TestConfmapProviderFactories(t *testing.T) {
	var schemes []string
	for _, factory := range ConfmapProviderFactories() {
		schemes = append(schemes, factory.Create(confmap.ProviderSettings{}).Scheme())
	}
	assert.Equal(t, []string{"env", "file", "http", "https", "yaml"}, schemes)
}

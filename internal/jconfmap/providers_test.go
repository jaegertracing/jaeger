// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jconfmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
)

func TestProviderFactories(t *testing.T) {
	var schemes []string
	for _, factory := range ProviderFactories() {
		schemes = append(schemes, factory.Create(confmap.ProviderSettings{}).Scheme())
	}
	assert.Equal(t, []string{"env", "file", "http", "https", "yaml"}, schemes)
}

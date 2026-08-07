// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiresServiceName_SearchCapabilities(t *testing.T) {
	// The embeddable default declares no optional query shapes, so a backend that
	// gains one has to say so explicitly rather than inherit it.
	assert.Equal(t, SearchCapabilities{}, RequiresServiceName{}.SearchCapabilities())
	assert.False(t, RequiresServiceName{}.SearchCapabilities().WithoutServiceName)
}

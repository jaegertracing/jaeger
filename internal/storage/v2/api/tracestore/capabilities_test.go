// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoSearchCapabilities_SearchCapabilities(t *testing.T) {
	// The embeddable default declares no optional query shapes, so a backend that
	// gains one has to say so explicitly rather than inherit it.
	assert.Equal(t, SearchCapabilities{}, NoSearchCapabilities{}.SearchCapabilities())
}

// TestSearchCapabilities_FieldCount is a tripwire rather than a property. The
// decorators that wrap a Reader are tested by enumerating every permutation of these
// fields, so a field added here without extending those tables would leave the new
// capability's forwarding unproven. Update the count with those tables.
func TestSearchCapabilities_FieldCount(t *testing.T) {
	assert.Equal(t, 1, reflect.TypeOf(SearchCapabilities{}).NumField(),
		"extend the permutation tables in the tracestoremetrics and queryinterceptor "+
			"reader-decorator tests, then update this count")
}

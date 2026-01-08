// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndexUtils_IndexWithDate(t *testing.T) {
	// this test covers the deprecated function to ensure code coverage
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	name := indexWithDate("jaeger-span-", "2006-01-02", date)
	assert.Equal(t, "jaeger-span-2025-01-01", name)
}

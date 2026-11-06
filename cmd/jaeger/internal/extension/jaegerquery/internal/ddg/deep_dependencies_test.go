// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package deepdependencies

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestGetData(t *testing.T) {
	data := GetData("")
	require.NotEmpty(t, data.Dependencies)
	// With no focal service the whole payload must be obviously synthetic (#6606).
	for _, path := range data.Dependencies {
		for _, entry := range path.Path {
			assert.True(t, strings.HasPrefix(entry.Service, "sample-"),
				"service %q should be an obvious sample identifier", entry.Service)
			assert.True(t, strings.HasPrefix(entry.Operation, "sample-"),
				"operation %q should be an obvious sample identifier", entry.Operation)
		}
		for _, attr := range path.Attributes {
			assert.True(t, strings.HasPrefix(attr.Value, "sample-"),
				"attribute %q=%q should be an obvious sample identifier", attr.Key, attr.Value)
		}
	}
}

func TestGetDataFocalServiceIsCentered(t *testing.T) {
	// The queried service is kept as the focal node so the UI centers on it, but
	// every neighbour stays an obvious sample so the graph is not mistaken for real.
	data := GetData("my-real-service")
	require.NotEmpty(t, data.Dependencies)
	focalSeen := false
	for _, path := range data.Dependencies {
		for _, entry := range path.Path {
			if entry.Service == "my-real-service" {
				focalSeen = true
				continue
			}
			assert.True(t, strings.HasPrefix(entry.Service, "sample-"),
				"non-focal service %q should be an obvious sample identifier", entry.Service)
		}
	}
	assert.True(t, focalSeen, "the queried focal service should appear as the focal node")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

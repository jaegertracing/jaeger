// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2/querysvc"
)

// mockExtension implements Extension for testing
type mockExtension struct {
	extension.Extension
	qs *querysvc.QueryService
}

func (m *mockExtension) QueryService() *querysvc.QueryService {
	return m.qs
}

func TestGetExtension_Success(t *testing.T) {
	// Create a mock QueryService
	mockQS := &querysvc.QueryService{}
	mockExt := &mockExtension{qs: mockQS}

	// Create a mock host with the jaegerquery extension
	host := &mockHost{
		Host: componenttest.NewNopHost(),
		ext:  mockExt,
	}

	// Get the extension
	ext, err := GetExtension(host)
	require.NoError(t, err)
	require.NotNil(t, ext)

	// Verify we got the right extension
	qs := ext.QueryService()
	assert.Equal(t, mockQS, qs)
}

func TestGetExtension_NotFound(t *testing.T) {
	// Create a mock host without the jaegerquery extension
	host := componenttest.NewNopHost()

	// Try to get the extension
	ext, err := GetExtension(host)
	require.Error(t, err)
	assert.Nil(t, ext)
	assert.Contains(t, err.Error(), "cannot find extension")
}

func TestGetExtension_WrongType(t *testing.T) {
	// Create a mock host with a component that isn't an Extension
	host := &mockHostWrongType{
		Host: componenttest.NewNopHost(),
	}

	// Try to get the extension
	ext, err := GetExtension(host)
	require.Error(t, err)
	assert.Nil(t, ext)
	assert.Contains(t, err.Error(), "not of expected type")
}

// mockHost implements component.Host for testing
type mockHost struct {
	component.Host
	ext Extension
}

func (m *mockHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		ID: m.ext,
	}
}

// mockHostWrongType implements component.Host with wrong type for testing
type mockHostWrongType struct {
	component.Host
}

func (m *mockHostWrongType) GetExtensions() map[component.ID]component.Component {
	// Return a component that's not an Extension
	return map[component.ID]component.Component{
		ID: &wrongTypeComponent{},
	}
}

// wrongTypeComponent implements component.Component but not Extension
type wrongTypeComponent struct{}

func (w *wrongTypeComponent) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (w *wrongTypeComponent) Shutdown(_ context.Context) error {
	return nil
}

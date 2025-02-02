// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ilm

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
)

func mockTestingClient(mockFxn func(cl *mocks.Client)) es.Client {
	c := &mocks.Client{}
	mockFxn(c)
	return c
}

func mockIlmPolicyExists(exists bool) es.Client {
	return mockTestingClient(func(cl *mocks.Client) {
		cl.On("IlmPolicyExists", mock.Anything, DefaultIlmPolicy).Return(exists, nil)
	})
}

func TestCreatePolicyIfNotExists(t *testing.T) {
	t.Run("IlmPolicyExists", func(t *testing.T) {
		cl := mockIlmPolicyExists(true)
		err := CreatePolicyIfNotExists(cl, false, 7)
		require.NoError(t, err)
	})
}

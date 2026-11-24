// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestGetCapabilities(t *testing.T) {
	tsc := newTestServerClientWithCapabilities(t, tracestore.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan, expression.LevelResource},
			Operators: []expression.Operator{expression.OpAnd, expression.OpEq},
		},
		Paginated:   true,
		SpanSearch:  true,
		SpanSorting: true,
	})

	response, err := tsc.caps.GetCapabilities(context.Background(), &api_v3.GetCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Equal(t, &api_v3.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter: &api_v3.FilterCapabilities{
			Levels:    []string{"span", "resource"},
			Operators: []string{"and", "eq"},
		},
		Paginated:   true,
		SpanSearch:  true,
		SpanSorting: true,
	}, response.GetSearch())
}

func TestGetCapabilitiesLeastCapable(t *testing.T) {
	tsc := newTestServerClient(t)

	response, err := tsc.caps.GetCapabilities(context.Background(), &api_v3.GetCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Equal(t, &api_v3.SearchCapabilities{}, response.GetSearch())
}

func TestGetCapabilitiesReaderErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{
			name: "a reader that cannot report is Unimplemented",
			err:  fmt.Errorf("cannot ask: %w", errors.ErrUnsupported),
			code: codes.Unimplemented,
		},
		{
			name: "any other failure is Internal",
			err:  assert.AnError,
			code: codes.Internal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tsc := newTestServerClient(t)
			// Replace the baseline declaration with the failure under test.
			tsc.reader.ExpectedCalls = nil
			tsc.reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, tt.err)

			response, err := tsc.caps.GetCapabilities(context.Background(), &api_v3.GetCapabilitiesRequest{})
			assert.Nil(t, response)
			assert.Equal(t, tt.code, status.Code(err))
			assert.ErrorContains(t, err, tt.err.Error())
		})
	}
}

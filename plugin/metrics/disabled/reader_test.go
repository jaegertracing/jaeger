// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package disabled

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

func TestGetLatencies(t *testing.T) {
	reader, err := NewMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricsstore.LatenciesQueryParameters{}
	r, err := reader.GetLatencies(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
	assert.EqualError(t, err, ErrDisabled.Error())
}

func TestGetCallRates(t *testing.T) {
	reader, err := NewMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricsstore.CallRateQueryParameters{}
	r, err := reader.GetCallRates(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
	assert.EqualError(t, err, ErrDisabled.Error())
}

func TestGetErrorRates(t *testing.T) {
	reader, err := NewMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricsstore.ErrorRateQueryParameters{}
	r, err := reader.GetErrorRates(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
	assert.EqualError(t, err, ErrDisabled.Error())
}

func TestGetMinStepDurations(t *testing.T) {
	reader, err := NewMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	qParams := &metricsstore.MinStepDurationQueryParameters{}
	r, err := reader.GetMinStepDuration(context.Background(), qParams)
	assert.Zero(t, r)
	assert.True(t, errors.Is(err, ErrDisabled))
	assert.EqualError(t, err, ErrDisabled.Error())
}

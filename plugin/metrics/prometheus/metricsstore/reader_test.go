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

package metricsstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

func TestNewMetricsReader(t *testing.T) {
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, nil, time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestGetMinStepDuration(t *testing.T) {
	params := metricsstore.MinStepDurationQueryParameters{}
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, nil, time.Second)
	assert.NoError(t, err)

	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	assert.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}

func TestGetLatencies(t *testing.T) {
	params := metricsstore.LatenciesQueryParameters{}
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, nil, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetLatencies(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

func TestGetCallRates(t *testing.T) {
	params := metricsstore.CallRateQueryParameters{}
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, nil, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetCallRates(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

func TestGetErrorRates(t *testing.T) {
	params := metricsstore.ErrorRateQueryParameters{}
	logger := zap.NewNop()
	reader, err := NewMetricsReader(logger, nil, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetErrorRates(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

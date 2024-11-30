// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstoremetrics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	protometrics "github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore/mocks"
)

func TestSuccessfulUnderlyingCalls(t *testing.T) {
	mf := metricstest.NewFactory(0)

	mockReader := mocks.Reader{}
	mrs := metricstoremetrics.NewReaderDecorator(&mockReader, mf)
	glParams := &metricsstore.LatenciesQueryParameters{}
	mockReader.On("GetLatencies", context.Background(), glParams).
		Return(&protometrics.MetricFamily{}, nil)
	mrs.GetLatencies(context.Background(), glParams)

	gcrParams := &metricsstore.CallRateQueryParameters{}
	mockReader.On("GetCallRates", context.Background(), gcrParams).
		Return(&protometrics.MetricFamily{}, nil)
	mrs.GetCallRates(context.Background(), gcrParams)

	gerParams := &metricsstore.ErrorRateQueryParameters{}
	mockReader.On("GetErrorRates", context.Background(), gerParams).
		Return(&protometrics.MetricFamily{}, nil)
	mrs.GetErrorRates(context.Background(), gerParams)

	msdParams := &metricsstore.MinStepDurationQueryParameters{}
	mockReader.On("GetMinStepDuration", context.Background(), msdParams).
		Return(time.Second, nil)
	mrs.GetMinStepDuration(context.Background(), msdParams)

	counters, gauges := mf.Snapshot()
	wantCounts := map[string]int64{
		"requests|operation=get_latencies|result=ok":          1,
		"requests|operation=get_latencies|result=err":         0,
		"requests|operation=get_call_rates|result=ok":         1,
		"requests|operation=get_call_rates|result=err":        0,
		"requests|operation=get_error_rates|result=ok":        1,
		"requests|operation=get_error_rates|result=err":       0,
		"requests|operation=get_min_step_duration|result=ok":  1,
		"requests|operation=get_min_step_duration|result=err": 0,
	}

	// This is not exhaustive.
	wantExistingKeys := []string{
		"latency|operation=get_latencies|result=ok.P50",
		"latency|operation=get_error_rates|result=ok.P50",
	}

	// This is not exhaustive.
	wantNonExistentKeys := []string{
		"latency|operation=get_latencies|result=err.P50",
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, wantCounts, gauges, wantExistingKeys, wantNonExistentKeys)
}

func checkExpectedExistingAndNonExistentCounters(t *testing.T,
	actualCounters,
	expectedCounters,
	actualGauges map[string]int64,
	existingKeys,
	nonExistentKeys []string,
) {
	for k, v := range expectedCounters {
		assert.EqualValues(t, v, actualCounters[k], k)
	}

	for _, k := range existingKeys {
		_, ok := actualGauges[k]
		assert.True(t, ok)
	}

	for _, k := range nonExistentKeys {
		_, ok := actualGauges[k]
		assert.False(t, ok)
	}
}

func TestFailingUnderlyingCalls(t *testing.T) {
	mf := metricstest.NewFactory(0)

	mockReader := mocks.Reader{}
	mrs := metricstoremetrics.NewReaderDecorator(&mockReader, mf)
	glParams := &metricsstore.LatenciesQueryParameters{}
	mockReader.On("GetLatencies", context.Background(), glParams).
		Return(&protometrics.MetricFamily{}, errors.New("failure"))
	mrs.GetLatencies(context.Background(), glParams)

	gcrParams := &metricsstore.CallRateQueryParameters{}
	mockReader.On("GetCallRates", context.Background(), gcrParams).
		Return(&protometrics.MetricFamily{}, errors.New("failure"))
	mrs.GetCallRates(context.Background(), gcrParams)

	gerParams := &metricsstore.ErrorRateQueryParameters{}
	mockReader.On("GetErrorRates", context.Background(), gerParams).
		Return(&protometrics.MetricFamily{}, errors.New("failure"))
	mrs.GetErrorRates(context.Background(), gerParams)

	msdParams := &metricsstore.MinStepDurationQueryParameters{}
	mockReader.On("GetMinStepDuration", context.Background(), msdParams).
		Return(time.Second, errors.New("failure"))
	mrs.GetMinStepDuration(context.Background(), msdParams)

	counters, gauges := mf.Snapshot()
	wantCounts := map[string]int64{
		"requests|operation=get_latencies|result=ok":          0,
		"requests|operation=get_latencies|result=err":         1,
		"requests|operation=get_call_rates|result=ok":         0,
		"requests|operation=get_call_rates|result=err":        1,
		"requests|operation=get_error_rates|result=ok":        0,
		"requests|operation=get_error_rates|result=err":       1,
		"requests|operation=get_min_step_duration|result=ok":  0,
		"requests|operation=get_min_step_duration|result=err": 1,
	}

	// This is not exhaustive.
	wantExistingKeys := []string{
		"latency|operation=get_latencies|result=err.P50",
	}

	// This is not exhaustive.
	wantNonExistentKeys := []string{
		"latency|operation=get_latencies|result=ok.P50",
		"latency|operation=get_error_rates|result=ok.P50",
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, wantCounts, gauges, wantExistingKeys, wantNonExistentKeys)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

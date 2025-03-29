// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

func TestToDomainMetricsFamily(t *testing.T) {
	promMetrics := model.Matrix{}
	nowSec := time.Now().Unix()
	promMetrics = append(promMetrics, &model.SampleStream{
		Metric: map[model.LabelName]model.LabelValue{"label_key": "label_value", "span_name": "span_name_value"},
		Values: []model.SamplePair{
			{Timestamp: model.Time(nowSec * 1000), Value: 1234},
		},
	})
	translator := New("span_name")
	mf, err := translator.ToDomainMetricsFamily("the_metric_name", "the_metric_description", promMetrics)
	require.NoError(t, err)

	assert.NotEmpty(t, mf)

	assert.Equal(t, "the_metric_name", mf.Name)
	assert.Equal(t, "the_metric_description", mf.Help)
	assert.Equal(t, metrics.MetricType_GAUGE, mf.Type)

	wantMetricLabels := map[string]string{
		"label_key": "label_value",
		"operation": "span_name_value", // assert the name is translated to a Jaeger-friendly label.
	}
	assert.Len(t, mf.Metrics, 1)
	for _, ml := range mf.Metrics[0].Labels {
		v, ok := wantMetricLabels[ml.Name]
		require.True(t, ok)
		assert.Equal(t, v, ml.Value)
		delete(wantMetricLabels, ml.Name)
	}
	assert.Empty(t, wantMetricLabels)

	wantMpValue := &metrics.MetricPoint_GaugeValue{
		GaugeValue: &metrics.GaugeValue{
			Value: &metrics.GaugeValue_DoubleValue{
				DoubleValue: 1234,
			},
		},
	}
	assert.Equal(t, []*metrics.MetricPoint{{Timestamp: &types.Timestamp{Seconds: nowSec}, Value: wantMpValue}}, mf.Metrics[0].MetricPoints)
}

func TestUnexpectedMetricsFamilyType(t *testing.T) {
	promMetrics := model.Vector{}
	translator := New("span_name")
	mf, err := translator.ToDomainMetricsFamily("the_metric_name", "the_metric_description", promMetrics)

	assert.NotNil(t, mf)
	assert.Empty(t, mf)

	require.Error(t, err)
	require.EqualError(t, err, "unexpected metrics ValueType: vector")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

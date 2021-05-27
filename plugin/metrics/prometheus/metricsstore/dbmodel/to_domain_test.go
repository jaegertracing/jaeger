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

package dbmodel

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

func TestToDomainMetricsFamily(t *testing.T) {
	promMetrics := model.Matrix{}
	nowSec := time.Now().Unix()
	promMetrics = append(promMetrics, &model.SampleStream{
		Metric: map[model.LabelName]model.LabelValue{"label_key": "label_value"},
		Values: []model.SamplePair{
			{Timestamp: model.Time(nowSec * 1000), Value: 1234},
		},
	})
	mf, err := ToDomainMetricsFamily("the_metric_name", "the_metric_description", promMetrics)
	require.NoError(t, err)

	assert.NotEmpty(t, mf)

	assert.Equal(t, "the_metric_name", mf.Name)
	assert.Equal(t, "the_metric_description", mf.Help)
	assert.Equal(t, metrics.MetricType_GAUGE, mf.Type)

	assert.Len(t, mf.Metrics, 1)
	assert.Equal(t, []*metrics.Label{{Name: "label_key", Value: "label_value"}}, mf.Metrics[0].Labels)

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
	mf, err := ToDomainMetricsFamily("the_metric_name", "the_metric_description", promMetrics)

	assert.NotNil(t, mf)
	assert.Empty(t, mf)

	require.Error(t, err)
	assert.EqualError(t, err, "unexpected metrics ValueType: vector")
}

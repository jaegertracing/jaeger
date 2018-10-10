// Copyright (c) 2018 The Jaeger Authors.
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

package consumer

import (
	"strconv"

	"github.com/uber/jaeger-lib/metrics"
)

type msgMetrics struct {
	counter     metrics.Counter
	offsetGauge metrics.Gauge
	lagGauge    metrics.Gauge
}

type errMetrics struct {
	errCounter metrics.Counter
}

type partitionMetrics struct {
	startCounter metrics.Counter
	closeCounter metrics.Counter
}

func (c *Consumer) namespace(partition int32) metrics.Factory {
	return c.metricsFactory.Namespace("sarama-consumer", map[string]string{"partition": strconv.Itoa(int(partition))})
}

func (c *Consumer) newMsgMetrics(partition int32) msgMetrics {
	f := c.namespace(partition)
	return msgMetrics{
		counter:     f.Counter("messages", nil),
		offsetGauge: f.Gauge("current-offset", nil),
		lagGauge:    f.Gauge("offset-lag", nil),
	}
}

func (c *Consumer) newErrMetrics(partition int32) errMetrics {
	return errMetrics{errCounter: c.namespace(partition).Counter("errors", nil)}
}

func (c *Consumer) partitionMetrics(partition int32) partitionMetrics {
	f := c.namespace(partition)
	return partitionMetrics{
		closeCounter: f.Counter("partition-close", nil),
		startCounter: f.Counter("partition-start", nil)}
}

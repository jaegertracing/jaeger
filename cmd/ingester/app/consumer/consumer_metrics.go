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

const consumerNamespace = "sarama-consumer"

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
	return c.metricsFactory.Namespace(metrics.NSOptions{Name: consumerNamespace, Tags: map[string]string{"partition": strconv.Itoa(int(partition))}})
}

func (c *Consumer) newMsgMetrics(partition int32) msgMetrics {
	f := c.namespace(partition)
	return msgMetrics{
		counter:     f.Counter(metrics.Options{Name: "messages", Tags: nil}),
		offsetGauge: f.Gauge(metrics.Options{Name: "current-offset", Tags: nil}),
		lagGauge:    f.Gauge(metrics.Options{Name: "offset-lag", Tags: nil}),
	}
}

func (c *Consumer) newErrMetrics(partition int32) errMetrics {
	return errMetrics{errCounter: c.namespace(partition).Counter(metrics.Options{Name: "errors", Tags: nil})}
}

func (c *Consumer) partitionMetrics(partition int32) partitionMetrics {
	f := c.namespace(partition)
	return partitionMetrics{
		closeCounter: f.Counter(metrics.Options{Name: "partition-close", Tags: nil}),
		startCounter: f.Counter(metrics.Options{Name: "partition-start", Tags: nil})}
}

func partitionsHeld(metricsFactory metrics.Factory) metrics.Counter {
	return metricsFactory.Namespace(metrics.NSOptions{Name: consumerNamespace, Tags: nil}).Counter(metrics.Options{Name: "partitions-held", Tags: nil})
}

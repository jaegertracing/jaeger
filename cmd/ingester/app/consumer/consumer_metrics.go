// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"strconv"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
)

const consumerNamespace = "sarama-consumer"

type msgMetrics struct {
	counter     api.Counter
	offsetGauge api.Gauge
	lagGauge    api.Gauge
}

type errMetrics struct {
	errCounter api.Counter
}

type partitionMetrics struct {
	startCounter api.Counter
	closeCounter api.Counter
}

func (c *Consumer) namespace(topic string, partition int32) api.Factory {
	return c.metricsFactory.Namespace(
		api.NSOptions{
			Name: consumerNamespace,
			Tags: map[string]string{
				"topic":     topic,
				"partition": strconv.Itoa(int(partition)),
			},
		})
}

func (c *Consumer) newMsgMetrics(topic string, partition int32) msgMetrics {
	f := c.namespace(topic, partition)
	return msgMetrics{
		counter:     f.Counter(api.Options{Name: "messages", Tags: nil}),
		offsetGauge: f.Gauge(api.Options{Name: "current-offset", Tags: nil}),
		lagGauge:    f.Gauge(api.Options{Name: "offset-lag", Tags: nil}),
	}
}

func (c *Consumer) newErrMetrics(topic string, partition int32) errMetrics {
	return errMetrics{errCounter: c.namespace(topic, partition).Counter(api.Options{Name: "errors", Tags: nil})}
}

func (c *Consumer) partitionMetrics(topic string, partition int32) partitionMetrics {
	f := c.namespace(topic, partition)
	return partitionMetrics{
		closeCounter: f.Counter(api.Options{Name: "partition-close", Tags: nil}),
		startCounter: f.Counter(api.Options{Name: "partition-start", Tags: nil}),
	}
}

func partitionsHeldGauge(metricsFactory api.Factory) api.Gauge {
	return metricsFactory.Namespace(api.NSOptions{Name: consumerNamespace, Tags: nil}).Gauge(api.Options{Name: "partitions-held", Tags: nil})
}

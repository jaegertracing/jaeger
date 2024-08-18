// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"strconv"

	"github.com/jaegertracing/jaeger/pkg/metrics"
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

func (c *Consumer) namespace(topic string, partition int32) metrics.Factory {
	return c.metricsFactory.Namespace(
		metrics.NSOptions{
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
		counter:     f.Counter(metrics.Options{Name: "messages", Tags: nil}),
		offsetGauge: f.Gauge(metrics.Options{Name: "current-offset", Tags: nil}),
		lagGauge:    f.Gauge(metrics.Options{Name: "offset-lag", Tags: nil}),
	}
}

func (c *Consumer) newErrMetrics(topic string, partition int32) errMetrics {
	return errMetrics{errCounter: c.namespace(topic, partition).Counter(metrics.Options{Name: "errors", Tags: nil})}
}

func (c *Consumer) partitionMetrics(topic string, partition int32) partitionMetrics {
	f := c.namespace(topic, partition)
	return partitionMetrics{
		closeCounter: f.Counter(metrics.Options{Name: "partition-close", Tags: nil}),
		startCounter: f.Counter(metrics.Options{Name: "partition-start", Tags: nil}),
	}
}

func partitionsHeldGauge(metricsFactory metrics.Factory) metrics.Gauge {
	return metricsFactory.Namespace(metrics.NSOptions{Name: consumerNamespace, Tags: nil}).Gauge(metrics.Options{Name: "partitions-held", Tags: nil})
}

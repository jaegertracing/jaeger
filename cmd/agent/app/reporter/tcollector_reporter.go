// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package reporter

import (
	"time"

	"github.com/uber-go/zap"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

const (
	jaegerBatches = "jaeger"
	zipkinBatches = "zipkin"
)

type batchMetrics struct {
	// Number of successful batch submissions to collector
	BatchesSubmitted metrics.Counter `metric:"batches.submitted"`

	// Number of failed batch submissions to collector
	BatchesFailures metrics.Counter `metric:"batches.failures"`

	// Number of spans in a batch submitted to collector
	BatchSize metrics.Gauge `metric:"batch_size"`

	// Number of successful span submissions to collector
	SpansSubmitted metrics.Counter `metric:"spans.submitted"`

	// Number of failed span submissions to collector
	SpansFailures metrics.Counter `metric:"spans.failures"`
}

// tcollectorReporter forwards received spans to central collector tier
type tcollectorReporter struct {
	zClient        zipkincore.TChanZipkinCollector
	jClient        jaeger.TChanCollector
	batchesMetrics map[string]batchMetrics
	logger         zap.Logger
}

// NewTCollectorReporter creates new tcollectorReporter
func NewTCollectorReporter(channel *tchannel.Channel, mFactory metrics.Factory, zlogger zap.Logger, clientOpts *thrift.ClientOptions) Reporter {
	thriftClient := thrift.NewClient(channel, "tcollector", clientOpts)
	zClient := zipkincore.NewTChanZipkinCollectorClient(thriftClient)
	jClient := jaeger.NewTChanCollectorClient(thriftClient)
	batchesMetrics := map[string]batchMetrics{}
	tcReporterNS := mFactory.Namespace("tc-reporter", nil)
	for _, s := range []string{zipkinBatches, jaegerBatches} {
		nsByType := tcReporterNS.Namespace(s, nil)
		bm := batchMetrics{}
		metrics.Init(&bm, nsByType, nil)
		batchesMetrics[s] = bm
	}
	rep := &tcollectorReporter{zClient: zClient, jClient: jClient, logger: zlogger, batchesMetrics: batchesMetrics}

	return rep
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *tcollectorReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	submissionFunc := func(ctx thrift.Context) error {
		_, err := r.zClient.SubmitZipkinBatch(ctx, spans)
		return err
	}
	return r.submitAndReport(
		submissionFunc,
		"Could not submit zipkin batch",
		int64(len(spans)),
		r.batchesMetrics[zipkinBatches],
	)
}

// EmitBatch implements EmitBatch() of Reporter
func (r *tcollectorReporter) EmitBatch(batch *jaeger.Batch) error {
	submissionFunc := func(ctx thrift.Context) error {
		_, err := r.jClient.SubmitBatches(ctx, []*jaeger.Batch{batch})
		return err
	}
	return r.submitAndReport(
		submissionFunc,
		"Could not submit jaeger batch",
		int64(len(batch.Spans)),
		r.batchesMetrics[jaegerBatches],
	)
}

func (r *tcollectorReporter) submitAndReport(submissionFunc func(ctx thrift.Context) error, errMsg string, size int64, batchMetrics batchMetrics) error {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).DisableTracing().Build()
	defer cancel()

	if err := submissionFunc(ctx); err != nil {
		batchMetrics.BatchesFailures.Inc(1)
		batchMetrics.SpansFailures.Inc(size)
		r.logger.Error(errMsg, zap.Error(err))
		return err
	}
	batchMetrics.BatchSize.Update(size)
	batchMetrics.BatchesSubmitted.Inc(1)
	batchMetrics.SpansSubmitted.Inc(size)
	return nil
}

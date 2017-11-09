// Copyright (c) 2017 Uber Technologies, Inc.
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

package tchannel

import (
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/discovery/peerlistmgr"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
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

// Reporter forwards received spans to central collector tier over TChannel.
type Reporter struct {
	channel        *tchannel.Channel
	zClient        zipkincore.TChanZipkinCollector
	jClient        jaeger.TChanCollector
	peerListMgr    *peerlistmgr.PeerListManager
	batchesMetrics map[string]batchMetrics
	logger         *zap.Logger
	mFactory       metrics.Factory
}

// New creates new TChannel-based Reporter.
func New(
	collectorServiceName string,
	channel *tchannel.Channel,
	peerListMgr *peerlistmgr.PeerListManager,
	mFactory metrics.Factory,
	zlogger *zap.Logger,
) *Reporter {
	thriftClient := thrift.NewClient(channel, collectorServiceName, nil)
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
	return &Reporter{
		channel:        channel,
		zClient:        zClient,
		jClient:        jClient,
		peerListMgr:    peerListMgr,
		logger:         zlogger,
		batchesMetrics: batchesMetrics,
		mFactory:       mFactory,
	}
}

// Channel returns the TChannel used by the reporter.
func (r *Reporter) Channel() *tchannel.Channel {
	return r.channel
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
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
func (r *Reporter) EmitBatch(batch *jaeger.Batch) error {
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

func (r *Reporter) submitAndReport(submissionFunc func(ctx thrift.Context) error, errMsg string, size int64, batchMetrics batchMetrics) error {
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

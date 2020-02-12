// Copyright (c) 2019 The Jaeger Authors.
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

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/discovery/peerlistmgr"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Reporter forwards received spans to central collector tier over TChannel.
type Reporter struct {
	channel       *tchannel.Channel
	zClient       zipkincore.TChanZipkinCollector
	jClient       jaeger.TChanCollector
	reportTimeout time.Duration
	peerListMgr   *peerlistmgr.PeerListManager
	logger        *zap.Logger
	serviceName   string
}

// New creates new TChannel-based Reporter.
func New(
	collectorServiceName string,
	channel *tchannel.Channel,
	reportTimeout time.Duration,
	peerListMgr *peerlistmgr.PeerListManager,
	zlogger *zap.Logger,
) *Reporter {
	thriftClient := thrift.NewClient(channel, collectorServiceName, nil)
	zClient := zipkincore.NewTChanZipkinCollectorClient(thriftClient)
	jClient := jaeger.NewTChanCollectorClient(thriftClient)
	return &Reporter{
		channel:       channel,
		zClient:       zClient,
		jClient:       jClient,
		reportTimeout: reportTimeout,
		peerListMgr:   peerListMgr,
		logger:        zlogger,
		serviceName:   collectorServiceName,
	}
}

// Channel returns the TChannel used by the reporter.
func (r *Reporter) Channel() *tchannel.Channel {
	return r.channel
}

// Close the underlying channel
func (r *Reporter) Close() error {
	return nil
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
	)
}

func (r *Reporter) submitAndReport(submissionFunc func(ctx thrift.Context) error, errMsg string, size int64) error {
	ctx, cancel := tchannel.NewContextBuilder(r.reportTimeout).DisableTracing().Build()
	defer cancel()

	if err := submissionFunc(ctx); err != nil {
		return err
	}

	r.logger.Debug("Span batch submitted by the agent", zap.Int64("span-count", size))
	return nil
}

// CollectorServiceName returns collector service name.
func (r *Reporter) CollectorServiceName() string {
	return r.serviceName
}

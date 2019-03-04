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
	agentTags	  []jaeger.Tag
}

// New creates new TChannel-based Reporter.
func New(
	collectorServiceName string,
	channel *tchannel.Channel,
	reportTimeout time.Duration,
	peerListMgr *peerlistmgr.PeerListManager,
	agentTagString map[string]string,
	zlogger *zap.Logger,
) *Reporter {
	thriftClient := thrift.NewClient(channel, collectorServiceName, nil)
	zClient := zipkincore.NewTChanZipkinCollectorClient(thriftClient)
	jClient := jaeger.NewTChanCollectorClient(thriftClient)
	agentTags := makeJaegerTag(agentTagString)
	return &Reporter{
		channel:       channel,
		zClient:       zClient,
		jClient:       jClient,
		reportTimeout: reportTimeout,
		peerListMgr:   peerListMgr,
		logger:        zlogger,
		serviceName:   collectorServiceName,
		agentTags:     agentTags,
	}
}

// Channel returns the TChannel used by the reporter.
func (r *Reporter) Channel() *tchannel.Channel {
	return r.channel
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	spans = addAgentTagsToZipkinBatch(spans, r.agentTags)
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
	batch.Spans = addAgentTags(batch.Spans, r.agentTags)
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

func addAgentTagsToZipkinBatch(spans []*zipkincore.Span, agentTags []jaeger.Tag) []*zipkincore.Span {
	for _, span := range spans {
		for _, tag := range agentTags{
			span.BinaryAnnotations = append(span.BinaryAnnotations, &zipkincore.BinaryAnnotation{
				Key: tag.Key,
				Value: []byte(*tag.VStr),
				AnnotationType: 6, // static value set to string type for now.
			})
		}
	}
	return spans
}

func addAgentTags(spans []*jaeger.Span, agentTags []jaeger.Tag) []*jaeger.Span {
	for _, span := range spans {
		for _, tag := range agentTags{
			span.Tags = append(span.Tags, &tag)
		}
	}
	return spans
}

func makeJaegerTag(agentTags map[string]string) []jaeger.Tag {
	tags := make([]jaeger.Tag, 0)
	for k, v := range agentTags {
		tag := jaeger.Tag{
			Key: k,
			VStr: &v,
		}
		tags = append(tags, tag)
	}

	return tags
}
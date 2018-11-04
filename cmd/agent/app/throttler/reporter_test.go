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

package throttling

import (
	"testing"
	"time"

	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	constants "github.com/uber/jaeger-client-go"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func makeDebugRootTag(tagType jaeger.TagType) *jaeger.Tag {
	tag := jaeger.NewTag()
	tag.Key = string(ext.SamplingPriority)
	tag.VType = tagType
	switch tagType {
	case jaeger.TagType_BOOL:
		tag.VBool = new(bool)
		*tag.VBool = true
	case jaeger.TagType_LONG:
		tag.VLong = new(int64)
		*tag.VLong = 12345
	case jaeger.TagType_DOUBLE:
		tag.VDouble = new(float64)
		*tag.VDouble = 1.2345
	case jaeger.TagType_STRING:
		tag.VStr = new(string)
		*tag.VStr = "1"
	case jaeger.TagType_BINARY:
		tag.VBinary = []byte("1")
	}
	return tag
}

func makeDebugRootSpan(tagType jaeger.TagType) *jaeger.Span {
	span := jaeger.NewSpan()
	span.Flags = int32(model.DebugFlag) | int32(model.SampledFlag)
	span.Tags = append(span.Tags, makeDebugRootTag(tagType))
	return span
}

func TestReporter(t *testing.T) {
	const (
		creditsPerSecond = 1.0
		numTagTypes      = 5
	)
	cfg := ThrottlerConfig{
		DefaultAccountConfig: AccountConfig{
			MaxOperations:    1,
			CreditsPerSecond: creditsPerSecond,
			MaxBalance:       numTagTypes,
		},
		AccountConfigOverrides: map[string]*AccountConfig{},
		ClientMaxBalance:       numTagTypes,
		InactiveEntryLifetime:  10 * time.Second,
		PurgeInterval:          100 * time.Millisecond,
		ConfigRefreshInterval:  100 * time.Millisecond,
	}
	startTime := time.Now()
	throttler := newTestingThrottler(t, cfg)
	defer throttler.Close()
	reporter := NewReporter(throttler)

	zipkinBatch := []*zipkincore.Span{nil, nil}
	err := reporter.EmitZipkinBatch(zipkinBatch)
	assert.Nil(t, err)

	const (
		serviceName  = "test-service"
		clientUUID   = "10"
		spendableMin = 1
	)
	batch := jaeger.NewBatch()
	batch.Process = jaeger.NewProcess()
	batch.Process.ServiceName = serviceName

	// Test submitting a batch lacking a client ID.
	span := makeDebugRootSpan(jaeger.TagType_STRING)
	span.OperationName = "a"
	batch.Spans = append(batch.Spans, span)
	err = reporter.EmitBatch(batch)
	assert.Error(t, err)
	assert.Equal(t, errNoClientID, err)

	clientUUIDTag := jaeger.NewTag()
	clientUUIDTag.Key = constants.TracerUUIDTagKey
	clientUUIDTag.VStr = new(string)
	*clientUUIDTag.VStr = clientUUID
	batch.Process.Tags = append(batch.Process.Tags, clientUUIDTag)

	// Test spending without withdrawing upfront.
	span = makeDebugRootSpan(jaeger.TagType_BOOL)
	span.OperationName = "a"
	batch.Spans = append(batch.Spans, span)
	err = reporter.EmitBatch(batch)
	assert.Error(t, err)
	assert.Regexp(t, "^\\[Overspending occurred: ", err.Error())

	// Rotate through different tag types.
	tagType := jaeger.TagType_STRING

	credits := throttler.Withdraw(serviceName, clientUUID, "a")
	batch.Spans = []*jaeger.Span{}
	for ; credits >= spendableMin; credits-- {
		span := makeDebugRootSpan(tagType)
		span.OperationName = "a"
		batch.Spans = append(batch.Spans, span)
		tagType = (tagType + 1) % numTagTypes
	}
	err = reporter.EmitBatch(batch)
	assert.NoError(t, err)
	client := throttler.clients[clientUUID]
	assert.NotNil(t, client)
	delta := time.Since(startTime).Seconds() * creditsPerSecond
	assert.InDelta(t, credits, client.perOperationBalance["a"], delta)

	// Make sure we are not throttling non-debug spans.
	nonDebugSpan := jaeger.NewSpan()
	nonDebugSpan.OperationName = "a"
	nonRootDebugSpan := jaeger.NewSpan()
	nonRootDebugSpan.OperationName = "a"
	nonRootDebugSpan.Flags = int32(model.DebugFlag) | int32(model.SampledFlag)
	batch.Spans = []*jaeger.Span{nonDebugSpan, nonRootDebugSpan}
	err = reporter.EmitBatch(batch)
	assert.NoError(t, err)

	// Check that debug span header is throttled
	debugSpanUsingHeader := jaeger.NewSpan()
	debugSpanUsingHeader.OperationName = "a"
	debugSpanUsingHeader.Flags = int32(model.DebugFlag) | int32(model.SampledFlag)
	headerTag := jaeger.NewTag()
	headerTag.Key = constants.JaegerDebugHeader
	headerTag.VType = jaeger.TagType_STRING
	headerTag.VStr = new(string)
	*headerTag.VStr = "x"
	debugSpanUsingHeader.Tags = append(debugSpanUsingHeader.Tags, headerTag)
	assert.NotEmpty(t, debugSpanUsingHeader)
	batch.Spans = []*jaeger.Span{debugSpanUsingHeader}
	assert.NotEmpty(t, batch.Spans[0].Tags)
	err = reporter.EmitBatch(batch)
	assert.Error(t, err)
}

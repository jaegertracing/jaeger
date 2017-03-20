package jaeger

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/model"
	j "github.com/uber/jaeger/thrift-gen/jaeger"
)

func spanRefsEqual(refs []*j.SpanRef, otherRefs []*j.SpanRef) bool {
	if len(refs) != len(otherRefs) {
		return false
	}

	for idx, ref := range refs {
		if *ref != *otherRefs[idx] {
			return false
		}
	}
	return true
}

type spanOptions struct {
	TraceIDLow    uint64
	TraceIDHigh   uint64
	SpanID        uint64
	ParentSpanID  uint64
	OperationName string
	References    []model.SpanRef
	Flags         int32
	StartTime     time.Time
	Duration      time.Duration
	Tags          model.KeyValues
	Logs          []model.Log
	Process       model.Process
}

func generateRandomSpan(options *spanOptions) *model.Span {
	if options == nil {
		options = &spanOptions{}
	}

	zeroTime := time.Time{}
	zeroDuration, _ := time.ParseDuration("")

	if options.TraceIDHigh == 0 {
		options.TraceIDHigh = rand.Uint64()
	}

	if options.TraceIDLow == 0 {
		options.TraceIDLow = rand.Uint64()
	}

	if options.SpanID == 0 {
		options.SpanID = rand.Uint64()
	}

	if options.ParentSpanID == 0 {
		options.ParentSpanID = rand.Uint64()
	}

	if options.OperationName == "" {
		options.OperationName = "someOperationName"
	}

	if options.StartTime == zeroTime {
		options.StartTime = model.EpochMicrosecondsAsTime(12345)
	}

	if options.Duration == zeroDuration {
		options.Duration = time.Duration(32) * time.Microsecond
	}

	if options.Process.ServiceName == "" {
		options.Process.ServiceName = "someServiceName"
	}

	if options.Process.Tags == nil {
		options.Process.Tags = []model.KeyValue{
			{Key: "client-version", VType: model.StringType, VStr: "golang-test"},
		}
	}

	return &model.Span{
		TraceID:       model.TraceID{High: options.TraceIDHigh, Low: options.TraceIDLow},
		SpanID:        model.SpanID(options.SpanID),
		ParentSpanID:  model.SpanID(options.ParentSpanID),
		OperationName: options.OperationName,
		References: []model.SpanRef{
			{
				TraceID: model.TraceID{High: options.TraceIDHigh, Low: options.TraceIDLow},
				SpanID:  model.SpanID(options.SpanID),
				RefType: model.ChildOf,
			},
		},
		Flags:     model.Flags(0),
		StartTime: options.StartTime,
		Duration:  options.Duration,
		Process: &model.Process{
			ServiceName: options.Process.ServiceName,
			Tags:        options.Process.Tags,
		},
	}
}

func generateRandomSpans(numSpans int) []*model.Span {
	spans := make([]*model.Span, numSpans)
	for i := 0; i < numSpans; i++ {
		spans[i] = generateRandomSpan(nil)
	}
	return spans
}

func TestFromDomainSpan(t *testing.T) {
	modelSpan := generateRandomSpan(nil)
	jaegerSpan := FromDomainSpan(modelSpan)

	assert.Equal(t, jaegerSpan.GetTraceIdLow(), int64(modelSpan.TraceID.Low))
	assert.Equal(t, jaegerSpan.GetTraceIdHigh(), int64(modelSpan.TraceID.High))
	assert.Equal(t, jaegerSpan.GetSpanId(), int64(modelSpan.SpanID))
	assert.Equal(t, jaegerSpan.GetParentSpanId(), int64(modelSpan.ParentSpanID))
	assert.Equal(t, jaegerSpan.GetOperationName(), modelSpan.OperationName)
	assert.Equal(t, uint64(jaegerSpan.StartTime), uint64(modelSpan.StartTime.Nanosecond()/1e3))
	assert.Equal(t, jaegerSpan.Duration, modelSpan.Duration.Nanoseconds()/1e3)
}

func TestFromDomain(t *testing.T) {
	numSpans := 2
	modelSpans := generateRandomSpans(numSpans)
	jaegerSpans := FromDomain(modelSpans)

	for i := 0; i < numSpans; i++ {
		modelSpan := modelSpans[i]
		jaegerSpan := jaegerSpans[i]

		assert.Equal(t, jaegerSpan.GetTraceIdLow(), int64(modelSpan.TraceID.Low))
		assert.Equal(t, jaegerSpan.GetTraceIdHigh(), int64(modelSpan.TraceID.High))
		assert.Equal(t, jaegerSpan.GetSpanId(), int64(modelSpan.SpanID))
		assert.Equal(t, jaegerSpan.GetParentSpanId(), int64(modelSpan.ParentSpanID))
		assert.Equal(t, jaegerSpan.GetOperationName(), modelSpan.OperationName)
		assert.Equal(t, uint64(jaegerSpan.StartTime), uint64(modelSpan.StartTime.Nanosecond()/1e3))
		assert.Equal(t, jaegerSpan.Duration, modelSpan.Duration.Nanoseconds()/1e3)
	}
}

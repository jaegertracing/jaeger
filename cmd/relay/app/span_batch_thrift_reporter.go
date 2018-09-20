package app

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	reporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/model"
	jConv "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	tmodel "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
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

// SpanBatchThriftReporter takes jaeger model batches and sends them
// out on tchannel
type SpanBatchThriftReporter struct {
	logger       *zap.Logger
	reporter     reporter.Reporter
	batchMetrics batchMetrics
}

// NewSpanBatchThriftReporter creates new TChannel-based Reporter.
func NewSpanBatchThriftReporter(
	reporter reporter.Reporter,
	mFactory metrics.Factory,
	zlogger *zap.Logger,
) *SpanBatchThriftReporter {
	bm := batchMetrics{}
	metrics.Init(&bm, mFactory.Namespace("span-batch-thrift-reporter", nil), nil)
	return &SpanBatchThriftReporter{
		logger:       zlogger,
		reporter:     reporter,
		batchMetrics: bm,
	}
}

// ProcessSpans implements SpanProcessor interface
func (s *SpanBatchThriftReporter) ProcessSpans(mSpans []*model.Span, spanFormat string) ([]bool, error) {
	tSpans := jConv.FromDomain(mSpans)
	tBatch := &tmodel.Batch{Spans: tSpans}
	oks := make([]bool, len(mSpans))
	if err := s.reporter.EmitBatch(tBatch); err != nil {
		s.logger.Error("Reporter failed to report span batch", zap.Error(err))
		for i := range oks {
			oks[i] = false
		}
		return oks, err
	}
	for i := range oks {
		oks[i] = true
	}
	return oks, nil
}

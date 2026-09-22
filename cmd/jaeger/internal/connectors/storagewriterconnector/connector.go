// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// connectorImpl is the traces→traces connector that writes each batch to storage
// and re-emits the spans the storage rejected terminally ("poison pills") onto its
// output pipeline, where the operator attaches any stock exporter as the dead-letter
// sink (RFC 0007 §4.8).
//
// The storage write runs inside an exporterhelper pipeline (the same one
// jaeger_storage_exporter is built on), so the connector's ConsumeTraces goes
// through the configured sending queue, blocking batcher, and retry policy before
// reaching writeTraces. That is what couples a Kafka offset to durability in the
// ingester (§4.2): a connector has no queue of its own, but wrapping the exporter
// pipeline gives it the exporter's. The outcome of writeTraces maps to the caller
// (and so to the Kafka offset) as follows:
//   - success                          → nil; the offset advances.
//   - a transport or transient error   → the error; the batch is retried, the offset held.
//   - only terminal rejections         → the poison spans go to the dead-letter
//     pipeline; if the sink accepts them → nil, the offset advances;
//     if the sink rejects them → an error, the offset held (§4.8 step 4).
//   - terminal and transient together  → the error; the whole batch is retried, and
//     the poison goes to the dead-letter pipeline once the transient failures
//     clear, so a retry never sends the same span there twice.
type connectorImpl struct {
	config   *Config
	logger   *zap.Logger
	writer   *storageexporter.TraceWriter
	exporter exporter.Traces // the queue/batch/retry pipeline in front of writeTraces
	next     consumer.Traces // the dead-letter pipeline
	// deadLetterSpans counts the spans handed to the dead-letter pipeline.
	deadLetterSpans metrics.Counter
}

func newConnector(ctx context.Context, set connector.Settings, cfg *Config, next consumer.Traces) (*connectorImpl, error) {
	c := &connectorImpl{
		config: cfg,
		logger: set.Logger,
		writer: storageexporter.NewTraceWriter(&cfg.Config, set.TelemetrySettings),
		next:   next,
		deadLetterSpans: otelmetrics.NewFactory(set.MeterProvider).
			Namespace(metrics.NSOptions{Name: "jaeger_storage_writer"}).
			Counter(metrics.Options{
				Name: "dead_letter_spans",
				Help: "Spans the storage rejected terminally that were sent to the dead-letter pipeline",
			}),
	}
	exp, err := exporterhelper.NewTraces(
		ctx,
		exporter.Settings{ID: set.ID, TelemetrySettings: set.TelemetrySettings, BuildInfo: set.BuildInfo},
		cfg,
		c.writeTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithRetry(cfg.RetryConfig),
		exporterhelper.WithQueue(cfg.QueueConfig),
		exporterhelper.WithStart(c.startWriter),
	)
	if err != nil {
		return nil, err
	}
	c.exporter = exp
	return c, nil
}

// Start starts the exporter pipeline, which resolves the storage through startWriter.
func (c *connectorImpl) Start(ctx context.Context, host component.Host) error {
	return c.exporter.Start(ctx, host)
}

// Shutdown drains and stops the exporter pipeline.
func (c *connectorImpl) Shutdown(ctx context.Context) error {
	return c.exporter.Shutdown(ctx)
}

func (c *connectorImpl) Capabilities() consumer.Capabilities {
	return c.exporter.Capabilities()
}

// ConsumeTraces hands the batch to the exporter pipeline and returns the write's
// verdict once the pipeline has one; with queue.wait_for_result that is after the
// batch it was merged into has been written (or sent to the dead-letter pipeline).
func (c *connectorImpl) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return c.exporter.ConsumeTraces(ctx, td)
}

// startWriter resolves the storage writer. The storage has to be one whose
// writer reports the spans it rejects terminally through a
// *tracestore.RejectedSpansError (for Elasticsearch/OpenSearch: write_mode: sync
// with poison_pill_handling: fail);
// against any other storage the connector still writes correctly but nothing ever
// reaches the dead-letter pipeline, so that requirement is documented rather than
// checked, as are the other settings of the at-least-once topology (RFC 0007 §4.5).
func (c *connectorImpl) startWriter(ctx context.Context, host component.Host) error {
	return c.writer.Start(ctx, host)
}

// writeTraces is the exporter pipeline's push function: it writes the batch and
// sends the terminally-rejected spans to the dead-letter pipeline. See
// connectorImpl for how its return value maps to the offset.
func (c *connectorImpl) writeTraces(ctx context.Context, td ptrace.Traces) error {
	err := c.writer.WriteTraces(ctx, td)
	if err == nil {
		return nil
	}
	var rejected *tracestore.RejectedSpansError
	if !errors.As(err, &rejected) || rejected.Transient {
		// Either the storage reports no per-span verdict (transport failure, backend
		// down) or some spans failed transiently: the batch has to be retried. Poison
		// spans ride along and are re-routed once a retry sees only terminal failures.
		return err
	}
	if rejected.Unidentified > 0 {
		// The storage rejected documents it could not attribute to a span, so they
		// cannot be re-routed. Acknowledging the batch could lose a span silently;
		// fail instead, so the batch is retried (forever, under the ingester's retry
		// policy) and the defect surfaces in the logs. Only a code or backend fix can
		// clear it, which is the right escape for a defect rather than a data problem.
		return fmt.Errorf("%d rejected documents could not be attributed to a span: %w", rejected.Unidentified, err)
	}
	// A storage may report the same span more than once; the first report wins for
	// both the attribute on the copy and the log line, so the two agree.
	unique := dedupeRejections(rejected.Spans)
	poison, unmatched := selectSpans(td, unique)
	if unmatched > 0 {
		// The storage named spans this batch does not contain, so they can be
		// neither re-routed nor acknowledged; fail the batch like an unidentified
		// document, so the defect surfaces instead of a span disappearing.
		return fmt.Errorf("%d rejected spans are not in the batch: %w", unmatched, err)
	}
	if n := poison.SpanCount(); n > 0 {
		if derr := c.next.ConsumeTraces(ctx, poison); derr != nil {
			// The dead-letter sink refused the spans, so nothing about them is
			// durable yet: hold the offset and retry the whole batch (§4.8 step 4).
			// The sink's error is rendered as text, not wrapped, on purpose: an
			// exporter marks a 4xx from its endpoint as consumererror permanent, and
			// a wrapped permanent error would make this connector's exporterhelper
			// skip its retries and hand the failure to the receiver, which pauses
			// the partition. From the storage write's point of view a sink failure
			// is always worth retrying, whatever the sink thought of it.
			return fmt.Errorf("dead-letter pipeline rejected %d poison spans: %s", n, derr.Error())
		}
		c.deadLetterSpans.Inc(int64(n))
		// Logged after the sink accepted, so a sink outage under an unbounded retry
		// policy does not repeat the lines on every attempt.
		for _, r := range unique {
			c.logger.Warn("storage rejected span terminally; sent it to the dead-letter pipeline",
				zap.Stringer("trace_id", r.TraceID), zap.Stringer("span_id", r.SpanID), zap.String("reason", r.Reason))
		}
	}
	// Every rejected document was a span and is now in the dead-letter pipeline, or
	// the storage handled it itself: the batch is complete, so the offset advances
	// and the partition never blocks.
	return nil
}

// spanKey identifies a span by the ids the storage reports a rejection with.
type spanKey struct {
	traceID pcommon.TraceID
	spanID  pcommon.SpanID
}

// dedupeRejections returns the rejections with one entry per span, keeping the
// first report of a span the storage named more than once.
func dedupeRejections(rejected []tracestore.RejectedSpan) []tracestore.RejectedSpan {
	seen := make(map[spanKey]struct{}, len(rejected))
	unique := make([]tracestore.RejectedSpan, 0, len(rejected))
	for _, r := range rejected {
		key := spanKey{r.TraceID, r.SpanID}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, r)
	}
	return unique
}

// rejectionReasonAttribute is the attribute set on every span the connector sends
// to the dead-letter pipeline, carrying the storage's reason for rejecting it
// (RFC 0007 §4.8: each terminal item is re-emitted with its reason attached). The
// span in the input batch is left untouched.
const rejectionReasonAttribute = "jaeger.storage.rejection_reason"

// selectSpans returns a new ptrace.Traces holding only the spans of td that the
// storage rejected, each under a copy of its resource and scope and tagged with
// the rejection reason, and the number of distinct rejected ids that matched no
// span in td. A second span in the batch that shares the trace and span id (the
// shared-span model, RFC 0007 §4.7) is selected alongside the rejected one, which
// over-includes a stored span in the dead letter but never loses one; that sibling
// carries the rejected span's reason, the only one the storage reported. td is the
// batch as the caller sent it; the sanitizers the writer applies never change trace
// or span ids, so the storage's ids match it.
func selectSpans(td ptrace.Traces, rejected []tracestore.RejectedSpan) (out ptrace.Traces, unmatched int) {
	want := make(map[spanKey]string, len(rejected))
	for _, r := range rejected {
		want[spanKey{r.TraceID, r.SpanID}] = r.Reason
	}
	out = ptrace.NewTraces()
	if len(want) == 0 {
		return out, 0
	}
	matched := make(map[spanKey]struct{}, len(want))
	for _, rs := range td.ResourceSpans().All() {
		var outRS ptrace.ResourceSpans
		rsInit := false
		for _, ss := range rs.ScopeSpans().All() {
			var outSS ptrace.ScopeSpans
			ssInit := false
			for _, span := range ss.Spans().All() {
				key := spanKey{span.TraceID(), span.SpanID()}
				reason, ok := want[key]
				if !ok {
					continue
				}
				matched[key] = struct{}{}
				if !rsInit {
					outRS = out.ResourceSpans().AppendEmpty()
					rs.Resource().CopyTo(outRS.Resource())
					outRS.SetSchemaUrl(rs.SchemaUrl())
					rsInit = true
				}
				if !ssInit {
					outSS = outRS.ScopeSpans().AppendEmpty()
					ss.Scope().CopyTo(outSS.Scope())
					outSS.SetSchemaUrl(ss.SchemaUrl())
					ssInit = true
				}
				copied := outSS.Spans().AppendEmpty()
				span.CopyTo(copied)
				copied.Attributes().PutStr(rejectionReasonAttribute, reason)
			}
		}
	}
	return out, len(want) - len(matched)
}

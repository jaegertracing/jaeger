// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
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
//     the poison is dead-lettered once the transient failures clear, so a retry
//     never dead-letters the same span twice.
type connectorImpl struct {
	config   *Config
	logger   *zap.Logger
	writer   *storageexporter.TraceWriter
	exporter exporter.Traces // the queue/batch/retry pipeline in front of writeTraces
	next     consumer.Traces // the dead-letter pipeline
	// deadLetteredSpans counts the spans handed to the dead-letter pipeline.
	deadLetteredSpans metrics.Counter
}

func newConnector(ctx context.Context, set connector.Settings, cfg *Config, next consumer.Traces) (*connectorImpl, error) {
	c := &connectorImpl{
		config: cfg,
		logger: set.Logger,
		writer: storageexporter.NewTraceWriter(cfg, set.TelemetrySettings),
		next:   next,
		deadLetteredSpans: otelmetrics.NewFactory(set.MeterProvider).
			Namespace(metrics.NSOptions{Name: "jaeger_storage_writer"}).
			Counter(metrics.Options{
				Name: "dead_lettered_spans",
				Help: "Spans the storage rejected terminally that were re-emitted onto the dead-letter pipeline",
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
// batch it was merged into has been written (or dead-lettered).
func (c *connectorImpl) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return c.exporter.ConsumeTraces(ctx, td)
}

// startWriter resolves the storage writer and refuses a storage that will never report a
// poison pill: with such a backend the connector would silently degrade to
// jaeger_storage_exporter and the dead-letter pipeline would never receive a span.
func (c *connectorImpl) startWriter(ctx context.Context, host component.Host) error {
	f, err := jaegerstorage.GetTraceStoreFactory(c.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}
	reporting, ok := f.(tracestore.PoisonPillReporting)
	if !ok || !reporting.ReportsPoisonPills() {
		return fmt.Errorf("trace storage %q does not report rejected spans, so %s would never dead-letter anything; "+
			"it requires an Elasticsearch/OpenSearch backend with write_mode: sync and poison_pill_handling: fail (RFC 0007 §4.8)",
			c.config.TraceStorage, componentType)
	}
	return c.writer.Start(ctx, host)
}

// writeTraces is the exporter pipeline's push function: it writes the batch and
// dead-letters the terminally-rejected spans. See connectorImpl for how its return
// value maps to the offset.
func (c *connectorImpl) writeTraces(ctx context.Context, td ptrace.Traces) error {
	err := c.writer.WriteTraces(ctx, td)
	if err == nil {
		return nil
	}
	var bulkErr *esclient.BulkWriteError
	if !errors.As(err, &bulkErr) || bulkErr.Transient {
		// Either nothing was rejected per item (transport failure, backend down) or
		// some items failed transiently: the batch has to be retried. Poison items
		// ride along and are dead-lettered once a retry sees only terminal failures.
		return err
	}
	poison, unmapped := filterPoisonSpans(td, bulkErr.Terminal)
	if unmapped > 0 {
		// A rejected document without a span _id is a service/operation lookup
		// document; it has no span to re-emit. Its spans are durable and any later
		// span of the same service and operation re-emits it, so it is only logged.
		c.logger.Warn("rejected documents that are not spans cannot be dead-lettered and were discarded",
			zap.Int("count", unmapped), zap.Error(bulkErr))
	}
	if n := poison.SpanCount(); n > 0 {
		if derr := c.next.ConsumeTraces(ctx, poison); derr != nil {
			// The dead-letter sink refused the spans, so nothing about them is
			// durable yet: hold the offset and retry the whole batch (§4.8 step 4).
			return fmt.Errorf("dead-letter pipeline rejected %d poison spans: %w", n, derr)
		}
		c.deadLetteredSpans.Inc(int64(n))
		c.logger.Warn("dead-lettered spans the storage rejected terminally",
			zap.Int("spans", n), zap.Error(bulkErr))
	}
	// Every rejected item was terminal and is now either dead-lettered or logged:
	// the batch is complete, so the offset advances and the partition never blocks.
	return nil
}

// filterPoisonSpans returns a new ptrace.Traces holding only the spans whose
// deterministic _id appears in the rejected items, each under a copy of its
// resource and scope, plus the number of rejected items that name no span. The
// span _id is traceID_spanID_hash (RFC 0007 §4.7), so its traceID_spanID prefix
// identifies the source span. A second span in the batch that shares the trace
// and span id (the shared-span model, §4.7) is re-emitted alongside the poison
// one, which over-includes a durable span in the dead letter but never loses one.
func filterPoisonSpans(td ptrace.Traces, rejected []esclient.RejectedItem) (poison ptrace.Traces, unmapped int) {
	want := make(map[string]struct{}, len(rejected))
	for _, it := range rejected {
		key, ok := spanKeyFromID(it.ID)
		if !ok {
			unmapped++
			continue
		}
		want[key] = struct{}{}
	}
	poison = ptrace.NewTraces()
	if len(want) == 0 {
		return poison, unmapped
	}
	for _, rs := range td.ResourceSpans().All() {
		var outRS ptrace.ResourceSpans
		rsInit := false
		for _, ss := range rs.ScopeSpans().All() {
			var outSS ptrace.ScopeSpans
			ssInit := false
			for _, span := range ss.Spans().All() {
				key := span.TraceID().String() + "_" + span.SpanID().String()
				if _, ok := want[key]; !ok {
					continue
				}
				if !rsInit {
					outRS = poison.ResourceSpans().AppendEmpty()
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
				span.CopyTo(outSS.Spans().AppendEmpty())
			}
		}
	}
	return poison, unmapped
}

// spanKeyFromID returns the traceID_spanID prefix of a span _id
// (traceID_spanID_hash). It reports false for an id without both delimiters, such
// as a service:operation document's, which names no span.
func spanKeyFromID(id string) (string, bool) {
	first := strings.IndexByte(id, '_')
	if first < 0 {
		return "", false
	}
	rest := id[first+1:]
	second := strings.IndexByte(rest, '_')
	if second < 0 {
		return "", false
	}
	return id[:first+1+second], true
}

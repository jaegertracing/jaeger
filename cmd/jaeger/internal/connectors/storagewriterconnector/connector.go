// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/jptrace/sanitizer"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// connectorImpl is a traces→traces connector that performs the synchronous storage
// write and taps terminally-rejected ("poison") spans onto a separate dead-letter
// pipeline (RFC 0007 §4.8). It is the connector shape the RFC left as an open
// prototype: unlike jaeger_storage_exporter (a terminal exporter), it has a
// downstream consumer, so it can re-emit the poison spans the storage write rejected.
//
// On each batch it calls WriteTraces exactly as the exporter does. The outcome maps
// to Kafka offset semantics as follows:
//   - success                       → return nil; the offset advances.
//   - a plain (transient) error     → return it; the batch is retried, the offset held.
//   - a *esclient.BulkWriteError    → dead-letter its terminal (poison) spans, then:
//   - all failures were terminal    → return nil after the sink accepts them; advance.
//   - transient failures remain     → return the error; retry the whole batch.
//   - the dead-letter sink rejects  → return an error; hold the offset (§4.8 step 4).
type connectorImpl struct {
	config      *Config
	logger      *zap.Logger
	next        consumer.Traces
	sanitizer   sanitizer.Func
	traceWriter tracestore.Writer

	// resolveWriter obtains the trace writer from the host's jaeger_storage extension.
	// It is a field so a unit test can substitute a fake writer without a live host.
	resolveWriter func(host component.Host) (tracestore.Writer, error)
}

func newConnector(config *Config, telemetry component.TelemetrySettings, next consumer.Traces) *connectorImpl {
	c := &connectorImpl{
		config:    config,
		logger:    telemetry.Logger,
		next:      next,
		sanitizer: sanitizer.Sanitize,
	}
	c.resolveWriter = func(host component.Host) (tracestore.Writer, error) {
		f, err := jaegerstorage.GetTraceStoreFactory(config.TraceStorage, host)
		if err != nil {
			return nil, fmt.Errorf("cannot find storage factory: %w", err)
		}
		w, err := f.CreateTraceWriter()
		if err != nil {
			return nil, fmt.Errorf("cannot create trace writer: %w", err)
		}
		return w, nil
	}
	return c
}

func (c *connectorImpl) Start(_ context.Context, host component.Host) error {
	w, err := c.resolveWriter(host)
	if err != nil {
		return err
	}
	c.traceWriter = w
	return nil
}

func (*connectorImpl) Shutdown(context.Context) error {
	// The trace writer is not closable; the jaeger_storage extension owns its lifecycle.
	return nil
}

func (*connectorImpl) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeTraces writes the batch to storage and, on a terminal-rejection error, taps
// the poison spans onto the connector's output pipeline. See connectorImpl's doc for
// the full outcome-to-offset mapping.
func (c *connectorImpl) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	td = c.sanitizer(td)
	err := c.traceWriter.WriteTraces(ctx, td)
	if err == nil {
		return nil
	}

	var bulkErr *esclient.BulkWriteError
	if !errors.As(err, &bulkErr) {
		// Not a per-item rejection (transport failure, whole-target-unavailable): the
		// whole batch is retryable. Hold the offset and let the pipeline re-send.
		return err
	}

	poison := filterPoisonSpans(td, bulkErr.Terminal)
	if poison.SpanCount() > 0 {
		if derr := c.next.ConsumeTraces(ctx, poison); derr != nil {
			// The dead-letter sink itself failed — treat as transient and hold the
			// offset; do not advance over spans we could not durably re-route (§4.8).
			return fmt.Errorf("dead-letter pipeline rejected %d poison spans: %w", poison.SpanCount(), derr)
		}
		c.logger.Warn("dead-lettered terminally-rejected spans",
			zap.Int("count", poison.SpanCount()))
	}

	if bulkErr.Transient {
		// Some failures were transient: the batch must be retried so the genuinely
		// missing (non-poison) spans are re-sent. Idempotent _ids make the already
		// durable and the already-dead-lettered spans no-ops on retry (§4.7).
		return err
	}
	// Every failure was terminal and has been dead-lettered: the batch is complete,
	// so return success and let the offset advance. The partition never blocks.
	return nil
}

// filterPoisonSpans builds a new ptrace.Traces containing only the spans whose
// deterministic _id appears in the terminal-rejection list, preserving each span's
// resource and scope. The _id is traceID_spanID_hash (RFC 0007 §4.7), so its
// traceID_spanID prefix maps a rejected bulk item back to its source span.
func filterPoisonSpans(td ptrace.Traces, terminal []esclient.RejectedItem) ptrace.Traces {
	want := make(map[string]struct{}, len(terminal))
	for _, it := range terminal {
		if key, ok := spanKeyFromID(it.ID); ok {
			want[key] = struct{}{}
		}
	}
	out := ptrace.NewTraces()
	if len(want) == 0 {
		return out
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
				span.CopyTo(outSS.Spans().AppendEmpty())
			}
		}
	}
	return out
}

// spanKeyFromID returns the traceID_spanID prefix of a deterministic span _id
// (traceID_spanID_hash). It reports false for an id without both delimiters — e.g. a
// service:operation doc, which is not a span and cannot be mapped back to one.
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

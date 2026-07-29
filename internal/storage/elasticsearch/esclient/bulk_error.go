// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"fmt"
	"strings"
)

// RejectedItem describes one document a synchronous _bulk write rejected
// terminally — a poison pill (a 4xx the backend rejects identically on replay:
// mapping conflict, malformed field, oversized field). It carries just enough for
// a caller to route the document to a dead-letter pipeline: ID is the document's
// deterministic _id (traceID_spanID_hash, RFC 0007 §4.7), which maps the rejection
// back to its source span, and Index/Status/Reason explain why it was rejected.
type RejectedItem struct {
	Index  string
	ID     string
	Status int
	Reason string
}

// BulkWriteError is the error SyncBulkWriter.WriteBatch returns when a batch had
// rejected items. Beyond the human-readable message (unchanged from the prior plain
// error), it carries structured per-item detail so a caller — the dead-letter
// connector (RFC 0007 §4.8) — can recover the terminally-rejected "poison" documents
// with errors.As and re-emit them onto a separate pipeline.
//
// The type survives the writer's own errors.Join across chunks and the pass-through
// up WriteSpans/WriteTraces (both return the writer's error unwrapped), so errors.As
// at the connector still finds it. WriteBatch aggregates every chunk's rejections
// into a single BulkWriteError rather than joining one per chunk, so the connector
// sees the whole batch's poison in one place.
//
// Terminal is populated only in `fail` mode (poison_pill_handling: fail). In `drop`
// mode the writer discards terminal poison itself before returning, so Terminal is
// empty and only Transient failures remain to retry — a connector wired to a
// drop-mode backend therefore dead-letters nothing, consistent with drop's contract.
type BulkWriteError struct {
	// Terminal lists the poison documents to dead-letter. Empty in drop mode.
	Terminal []RejectedItem
	// Transient is true if the batch also had transient (429/5xx/malformed-result)
	// failures. The caller must retry the whole batch when it is set; a re-sent
	// already-durable span is a no-op via its deterministic _id (§4.7).
	Transient bool

	// rejected/total/sample/overflow render the message and are unexported because
	// they are presentation detail, not part of the routing contract.
	rejected int
	total    int
	sample   []string
	overflow int
}

// Error renders "N of M bulk items rejected: <bounded sample>", matching the message
// the writer produced before the type was introduced so existing log/error
// expectations are unchanged.
func (e *BulkWriteError) Error() string {
	msg := strings.Join(e.sample, "; ")
	if e.overflow > 0 {
		msg += fmt.Sprintf("; …and %d more", e.overflow)
	}
	return fmt.Sprintf("%d of %d bulk items rejected: %s", e.rejected, e.total, msg)
}

// merge folds another chunk's BulkWriteError into e, summing the counts, unioning the
// terminal items and the transient flag, and keeping the rendered sample bounded to
// maxReportedFailures while accumulating the overflow that the sample omits.
func (e *BulkWriteError) merge(o *BulkWriteError) {
	e.Terminal = append(e.Terminal, o.Terminal...)
	e.Transient = e.Transient || o.Transient
	e.rejected += o.rejected
	e.total += o.total
	room := maxReportedFailures - len(e.sample)
	add := o.sample
	if len(add) > room {
		add = add[:room]
	}
	e.sample = append(e.sample, add...)
	// Whatever of o's sample we didn't keep, plus o's own already-dropped reasons,
	// becomes overflow so the "…and N more" count stays truthful.
	e.overflow += o.overflow + (len(o.sample) - len(add))
}

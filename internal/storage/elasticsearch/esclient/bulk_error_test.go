// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// TestBulkWriteError_FailModeCarriesTerminalItems proves the sync writer's fail-mode
// error is a *BulkWriteError carrying the terminally-rejected (poison) items with
// their _id, index, status and reason — the detail a dead-letter connector needs.
func TestBulkWriteError_FailModeCarriesTerminalItems(t *testing.T) {
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[` +
			`{"index":{"_index":"idx","status":201}},` + // durable
			`{"index":{"_index":"jaeger-span-1","_id":"tid_sid_deadbeef","status":400,"error":{"type":"mapper_parsing_exception","reason":"bad field"}}}` + // poison
			`]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.WriteBatch(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "jaeger-span-1", Body: map[string]any{"b": 2}},
	})
	require.Error(t, err)

	var be *BulkWriteError
	require.ErrorAs(t, err, &be, "fail mode must return a typed *BulkWriteError")
	assert.False(t, be.Transient, "a 400 is terminal, not transient")
	require.Len(t, be.Terminal, 1)
	assert.Equal(t, RejectedItem{
		Index:  "jaeger-span-1",
		ID:     "tid_sid_deadbeef",
		Status: 400,
		Reason: `{"type":"mapper_parsing_exception","reason":"bad field"}`,
	}, be.Terminal[0])
	// The message is preserved for logs/humans.
	assert.Contains(t, be.Error(), "1 of 2 bulk items rejected")
}

// TestBulkWriteError_SurvivesJoinAndPassThrough is the traversal proof required by
// the design: the type must be recoverable with errors.As after the writer's own
// errors.Join across chunks *and* after the pass-through wrapping that WriteSpans /
// WriteTraces do (they return the error unwrapped today; this also covers a future
// fmt.Errorf("%w") wrap).
func TestBulkWriteError_SurvivesJoinAndPassThrough(t *testing.T) {
	inner := &BulkWriteError{
		Terminal: []RejectedItem{{Index: "idx", ID: "a_b_c", Status: 400, Reason: "boom"}},
	}
	// errors.Join is exactly what WriteBatch applies, even to a single error.
	joined := errors.Join(inner)
	// A pass-through caller may additionally wrap with %w.
	wrapped := fmt.Errorf("write traces: %w", joined)

	var be *BulkWriteError
	require.ErrorAs(t, wrapped, &be)
	assert.Same(t, inner, be, "errors.As must recover the same instance through Join + %%w")
}

// TestBulkWriteError_AggregatesAcrossChunks forces a multi-chunk write (tiny byte
// cap) where every chunk rejects one poison item, and asserts the whole batch's
// poison is aggregated into a single *BulkWriteError rather than joined per chunk.
func TestBulkWriteError_AggregatesAcrossChunks(t *testing.T) {
	// A 20-byte cap sends each item in its own chunk (see the chunk-split test),
	// so the two poison items arrive as two separate _bulk responses.
	poison := func(id string) string {
		return `{"items":[{"index":{"_index":"idx","_id":"` + id + `","status":400,"error":{"reason":"x"}}}]}`
	}
	var call int
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		call++
		w.Write([]byte(poison(fmt.Sprintf("t_s_%d", call))))
	})
	w := newSyncWriter(t, url, 20, metrics.NullFactory, zap.NewNop())

	err := w.WriteBatch(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.Error(t, err)

	var be *BulkWriteError
	require.ErrorAs(t, err, &be)
	require.Len(t, be.Terminal, 2, "both chunks' poison items are aggregated into one error")
	assert.Equal(t, "t_s_1", be.Terminal[0].ID)
	assert.Equal(t, "t_s_2", be.Terminal[1].ID)
	assert.Contains(t, be.Error(), "2 of 2 bulk items rejected")
}

// TestBulkWriteError_TransientHasNoTerminalItems checks that a purely transient
// failure (429) sets Transient and carries no poison to dead-letter.
func TestBulkWriteError_TransientHasNoTerminalItems(t *testing.T) {
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{"index":{"_index":"idx","status":429,"error":{"reason":"busy"}}}]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.WriteBatch(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
	require.Error(t, err)

	var be *BulkWriteError
	require.ErrorAs(t, err, &be)
	assert.True(t, be.Transient)
	assert.Empty(t, be.Terminal, "a transient 429 is not poison")
}

// TestBulkWriteError_DropModeDoesNotDeadLetter confirms that in drop mode the writer
// discards terminal poison itself: the returned error (from a co-occurring transient
// failure) carries no Terminal items, so a connector wired to a drop-mode backend
// dead-letters nothing — consistent with drop's contract.
func TestBulkWriteError_DropModeDoesNotDeadLetter(t *testing.T) {
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[` +
			`{"index":{"_index":"idx","_id":"t_s_p","status":400,"error":{"reason":"poison"}}},` + // terminal → dropped
			`{"index":{"_index":"idx","status":503,"error":{"reason":"unavailable"}}}` + // transient → retry
			`]}`))
	})
	w := newSyncWriterDropPoison(t, url, metrics.NullFactory, zap.NewNop())

	err := w.WriteBatch(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.Error(t, err)

	var be *BulkWriteError
	require.ErrorAs(t, err, &be)
	assert.True(t, be.Transient)
	assert.Empty(t, be.Terminal, "drop mode discards poison, so nothing is dead-lettered")
}

// TestBulkWriteError_MergeBoundsSample checks that merging chunk errors keeps the
// rendered sample capped at maxReportedFailures while the overflow count stays
// truthful — the multi-chunk path where a later chunk's reasons are truncated.
func TestBulkWriteError_MergeBoundsSample(t *testing.T) {
	sampleOf := func(n int) []string {
		s := make([]string, n)
		for i := range s {
			s[i] = fmt.Sprintf("r%d", i)
		}
		return s
	}
	// First chunk already fills the sample to the cap; the second's reasons are all
	// truncated away but still counted as overflow.
	a := &BulkWriteError{rejected: maxReportedFailures, total: maxReportedFailures, sample: sampleOf(maxReportedFailures)}
	b := &BulkWriteError{
		Terminal: []RejectedItem{{ID: "t_s_x"}},
		rejected: 3, total: 3, sample: sampleOf(3),
	}
	a.merge(b)

	assert.Len(t, a.sample, maxReportedFailures, "the rendered sample never exceeds the cap")
	assert.Equal(t, maxReportedFailures+3, a.rejected)
	require.Len(t, a.Terminal, 1)
	assert.Contains(t, a.Error(), "…and 3 more")
}

// TestBulkWriteError_MessageOverflow exercises the "…and N more" rendering when the
// terminal sample exceeds maxReportedFailures, so the merge/overflow accounting is
// covered independently of the writer.
func TestBulkWriteError_MessageOverflow(t *testing.T) {
	const n = maxReportedFailures + 3
	items := make([]string, n)
	sent := make([]BulkItem, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"index":{"_index":"idx","_id":"t_s_%d","status":400,"error":{"reason":"y"}}}`, i)
		sent[i] = BulkItem{Index: "idx", Body: map[string]any{"a": i}}
	}
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[` + strings.Join(items, ",") + `]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.WriteBatch(context.Background(), sent)
	require.Error(t, err)
	var be *BulkWriteError
	require.ErrorAs(t, err, &be)
	require.Len(t, be.Terminal, n, "every poison item is captured, not just the sampled ones")
	assert.Contains(t, be.Error(), fmt.Sprintf("…and %d more", n-maxReportedFailures))
}

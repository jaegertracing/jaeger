// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
)

var (
	_ component.Config  = (*Config)(nil)
	_ confmap.Validator = (*Config)(nil)
)

// Config configures the connector form of jaeger_storage_exporter. It embeds the
// jaeger_storage_exporter configuration (trace_storage, queue, retry_on_failure),
// because the connector is that exporter's write path with a dead-letter output
// added: the same sending queue, blocking batcher, and retry policy apply to
// the storage write (RFC 0007 §4.2, §4.5).
//
// The named trace storage must report the spans it rejects terminally through a
// *tracestore.RejectedSpansError: for Elasticsearch/OpenSearch that is
// write_mode: sync with poison_pill_handling: fail. Against any other storage the connector writes
// like the exporter and nothing ever reaches the dead-letter pipeline.
type Config struct {
	storageexporter.Config `mapstructure:",squash"`
}

// errQueueWithoutResult rejects a queue that acknowledges spans on enqueue. The
// connector exists to return the storage write's verdict to its caller, so an
// enabled queue must block the caller until the write completes.
var errQueueWithoutResult = errors.New("queue.wait_for_result must be true: the connector returns the storage write's result to its caller, and a queue that acknowledges on enqueue would advance the receiver before the storage has the spans")

func (cfg *Config) Validate() error {
	if err := cfg.Config.Validate(); err != nil {
		return err
	}
	if cfg.QueueConfig.HasValue() && !cfg.QueueConfig.Get().WaitForResult {
		return errQueueWithoutResult
	}
	return nil
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"encoding/json"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

// BulkItem is a single document to be written via the bulk API. The caller
// always supplies Type; the indexer emits it only on typed-index backends (ES6),
// keeping the backend version an init-time concern the caller never sees.
type BulkItem struct {
	Index  string         // target index, alias, or data stream
	Type   string         // mapping type; emitted only on typed-index backends
	ID     string         // optional document _id (empty ⇒ server-generated)
	OpType es.WriteOpType // "index" (default) or "create"
	Body   any            // JSON-serializable source document
}

// encodeBulkItem appends item's NDJSON action/source line pair to buf. typed
// reports whether the backend supports typed indices (ES6); when false, _type is
// omitted. The result is two newline-terminated JSON lines, as the _bulk API
// requires.
func encodeBulkItem(buf *bytes.Buffer, item BulkItem, typed bool) error {
	meta := map[string]string{"_index": item.Index}
	if typed && item.Type != "" {
		meta["_type"] = item.Type
	}
	if item.ID != "" {
		meta["_id"] = item.ID
	}
	opType := item.OpType
	if opType == "" {
		opType = es.WriteOpIndex
	}
	// Emit the action line then the source line, each newline-terminated.
	for _, line := range []any{map[string]map[string]string{string(opType): meta}, item.Body} {
		encoded, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(encoded)
		buf.WriteByte('\n')
	}
	return nil
}

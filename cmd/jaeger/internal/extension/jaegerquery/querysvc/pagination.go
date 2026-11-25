// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"go.opentelemetry.io/collector/featuregate"
)

// PaginationGate admits the RFC 0014 Pagination field on a trace search and a page token on a
// span search. It is on by default, and a deployment that disables it refuses both, so an
// operator can withhold pagination until the trace searches of every backend page (RFC 0014 M3).
var PaginationGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.pagination",
	featuregate.StageBeta,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0014 Pagination field on a trace search and a page token on a span "+
			"search. A query that carries either is refused while this is disabled. Whether a "+
			"search continues past its first page depends on the storage backend: the trace "+
			"searches of Elasticsearch, OpenSearch and ClickHouse return a single page.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0014-search-result-pagination.md"),
)

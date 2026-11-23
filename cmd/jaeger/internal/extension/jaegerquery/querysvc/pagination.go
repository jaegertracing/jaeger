// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"go.opentelemetry.io/collector/featuregate"
)

// PaginationGate admits the RFC 0014 Pagination field on a trace search. While it is disabled, a
// span search returns no continuation token, since it would refuse one.
var PaginationGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.pagination",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0014 Pagination field on a trace search. While it is disabled, a span "+
			"search returns no continuation token, since it would refuse one.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0014-search-result-pagination.md"),
)

// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import "github.com/jaegertracing/jaeger/internal/storage/v1/metricstore"

// MetricsQueryService provides a means of querying R.E.D metrics from an underlying metrics store.
type MetricsQueryService interface {
	metricstore.Reader
}

// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metricsstore

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore/prometheus"
)

// Type is the metrics storage type.
const Type = "prometheus"

// MetricsReader is the reader for the M3 metrics backing store.
type MetricsReader struct {
	*prometheus.MetricsReader
}

// NewMetricsReader returns a new Prometheus MetricsReader, composing an underlying prometheus.MetricsReader.
func NewMetricsReader(logger *zap.Logger, hostPorts []string, connTimeout time.Duration) (metricsstore.Reader, error) {
	promReader, _ := prometheus.NewMetricsReader(Type, logger, hostPorts, connTimeout)
	return &MetricsReader{
		MetricsReader: promReader,
	}, nil
}

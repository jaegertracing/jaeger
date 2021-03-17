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

package memory

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"
)

// inMemoryExporter stores traces in memory.
type inMemoryExporter struct {
	name   string
	logger *zap.Logger

	stopCh   chan (struct{})
	stopped  bool
	stopLock sync.Mutex
}

// newExporter returns a new exporter for the in-memory storage.
func newExporter(cfg *Config, logger *zap.Logger) (component.TracesExporter, error) {
	s := &inMemoryExporter{
		name:   cfg.NameVal,
		logger: logger,

		stopCh: make(chan (struct{})),
	}

	return exporterhelper.NewTraceExporter(
		cfg,
		logger,
		s.pushTraceData,
		exporterhelper.WithStart(s.start),
		exporterhelper.WithShutdown(s.shutdown),
	)
}

func (s *inMemoryExporter) pushTraceData(ctx context.Context, td pdata.Traces) (droppedSpans int, err error) {
	return 0, nil
}

func (s *inMemoryExporter) shutdown(context.Context) error {
	s.stopLock.Lock()
	s.stopped = true
	s.stopLock.Unlock()
	close(s.stopCh)
	return nil
}

func (s *inMemoryExporter) start(context.Context, component.Host) error {
	return nil
}

// Copyright (c) 2020 The Jaeger Authors.
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

package uiconv

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	uimodel "github.com/jaegertracing/jaeger/model/json"
)

// Extractor reads the spans from reader, filters by traceID, and stores as JSON into uiFile.
type Extractor struct {
	uiFile  *os.File
	traceID string
	reader  *Reader
	logger  *zap.Logger
}

// NewExtractor creates Extractor.
func NewExtractor(uiFile string, traceID string, reader *Reader, logger *zap.Logger) (*Extractor, error) {
	f, err := os.OpenFile(uiFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing spans to UI file %s", uiFile)

	return &Extractor{
		uiFile:  f,
		traceID: traceID,
		reader:  reader,
		logger:  logger,
	}, nil
}

// Run executes the extraction.
func (e *Extractor) Run() error {
	e.logger.Info("Parsing captured file for trace", zap.String("trace_id", e.traceID))

	var (
		spans []uimodel.Span
		span  *uimodel.Span
		err   error
	)
	for span, err = e.reader.NextSpan(); err == nil; span, err = e.reader.NextSpan() {
		if string(span.TraceID) == e.traceID {
			spans = append(spans, *span)
		}
	}
	if err != io.EOF {
		return fmt.Errorf("failed when scanning the file: %w", err)
	}
	trace := uimodel.Trace{
		TraceID:   uimodel.TraceID(e.traceID),
		Spans:     spans,
		Processes: make(map[uimodel.ProcessID]uimodel.Process),
	}
	// (ys) The following is not exactly correct because it does not dedupe the processes,
	// but I don't think it affects the UI.
	for i := range spans {
		span := &spans[i]
		pid := uimodel.ProcessID(fmt.Sprintf("p%d", i))
		trace.Processes[pid] = *span.Process
		span.Process = nil
		span.ProcessID = pid
	}
	jsonBytes, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("failed to marshal UI trace: %w", err)
	}
	e.uiFile.Write([]byte(`{"data": [`))
	e.uiFile.Write(jsonBytes)
	e.uiFile.Write([]byte(`]}`))
	e.uiFile.Sync()
	e.uiFile.Close()
	e.logger.Sugar().Infof("Wrote spans to UI file %s", e.uiFile.Name())
	return nil
}

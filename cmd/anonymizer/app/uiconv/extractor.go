// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	uimodel "github.com/jaegertracing/jaeger/internal/converter/json"
)

// extractor reads the spans from reader, filters by traceID, and stores as JSON into uiFile.
type extractor struct {
	uiFile  *os.File
	traceID string
	reader  *spanReader
	logger  *zap.Logger
}

// newExtractor creates extractor.
func newExtractor(uiFile string, traceID string, reader *spanReader, logger *zap.Logger) (*extractor, error) {
	f, err := os.OpenFile(uiFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing spans to UI file %s", uiFile)

	return &extractor{
		uiFile:  f,
		traceID: traceID,
		reader:  reader,
		logger:  logger,
	}, nil
}

// Run executes the extraction.
func (e *extractor) Run() error {
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
	if !errors.Is(err, errNoMoreSpans) {
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

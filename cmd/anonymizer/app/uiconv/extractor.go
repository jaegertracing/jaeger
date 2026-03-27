// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/uimodel"
)

// extractor reads the spans from reader, filters by traceID, and stores as JSON into uiFile.
type extractor struct {
	uiFilePath string
	traceID    string
	reader     *spanReader
	logger     *zap.Logger
}

// newExtractor creates extractor.
func newExtractor(uiFile string, traceID string, reader *spanReader, logger *zap.Logger) (*extractor, error) {
	logger.Sugar().Infof("Writing spans to UI file %s", uiFile)
	return &extractor{
		uiFilePath: uiFile,
		traceID:    traceID,
		reader:     reader,
		logger:     logger,
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

	// Open (and truncate) the output file only after all processing succeeds,
	// so a previously valid output file is not destroyed if scanning or marshaling fails.
	f, err := os.OpenFile(e.uiFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("cannot create output file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(`{"data": [`); err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}
	if _, err := f.Write(jsonBytes); err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}
	if _, err := f.WriteString(`]}`); err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync output file: %w", err)
	}
	e.logger.Sugar().Infof("Wrote spans to UI file %s", e.uiFilePath)
	return nil
}

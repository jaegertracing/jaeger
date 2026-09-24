// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/anonymizer"
)

var ErrMaxSpansCountReached = errors.New("max spans count reached")

// Config contains parameters to NewWriter.
type Config struct {
	MaxSpansCount  int                `yaml:"max_spans_count" name:"max_spans_count"`
	CapturedFile   string             `yaml:"captured_file" name:"captured_file"`
	AnonymizedFile string             `yaml:"anonymized_file" name:"anonymized_file"`
	MappingFile    string             `yaml:"mapping_file" name:"mapping_file"`
	AnonymizerOpts anonymizer.Options `yaml:"anonymizer" name:"anonymizer"`
}

// Writer is a trace Writer that obfuscates the trace and writes it to a JSON file.
//
// Both files hold a single OTLP JSON document, which cannot be appended to span by span, so the
// traces are collected in memory and written out when the Writer is closed.
type Writer struct {
	config         Config
	lock           sync.Mutex
	logger         *zap.Logger
	capturedFile   *os.File
	anonymizedFile *os.File
	anonymizer     *anonymizer.Anonymizer
	captured       ptrace.Traces
	anonymized     ptrace.Traces
	spanCount      int
	closed         bool
}

// New creates an Writer
func New(config Config, logger *zap.Logger) (*Writer, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	logger.Sugar().Infof("Current working dir is %s", wd)

	cf, err := os.OpenFile(config.CapturedFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing captured spans to file %s", config.CapturedFile)

	af, err := os.OpenFile(config.AnonymizedFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing anonymized spans to file %s", config.AnonymizedFile)

	options := anonymizer.Options{
		HashStandardTags: config.AnonymizerOpts.HashStandardTags,
		HashCustomTags:   config.AnonymizerOpts.HashCustomTags,
		HashLogs:         config.AnonymizerOpts.HashLogs,
		HashProcess:      config.AnonymizerOpts.HashProcess,
	}

	return &Writer{
		config:         config,
		logger:         logger,
		capturedFile:   cf,
		anonymizedFile: af,
		anonymizer:     anonymizer.New(config.MappingFile, options, logger),
		captured:       ptrace.NewTraces(),
		anonymized:     ptrace.NewTraces(),
	}, nil
}

// WriteTraces anonymizes the traces and adds them, and the traces as captured, to the output.
func (w *Writer) WriteTraces(traces ptrace.Traces) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.closed {
		if w.config.MaxSpansCount > 0 && w.spanCount >= w.config.MaxSpansCount {
			return ErrMaxSpansCountReached
		}
		return errors.New("writer is closed")
	}

	captured := ptrace.NewTraces()
	traces.CopyTo(captured)
	if w.config.MaxSpansCount > 0 {
		truncate(captured, w.config.MaxSpansCount-w.spanCount)
	}
	spanCount := captured.SpanCount()

	anonymized := ptrace.NewTraces()
	captured.CopyTo(anonymized)
	w.anonymizer.AnonymizeTraces(anonymized)

	captured.ResourceSpans().MoveAndAppendTo(w.captured.ResourceSpans())
	anonymized.ResourceSpans().MoveAndAppendTo(w.anonymized.ResourceSpans())

	w.spanCount += spanCount
	w.logger.Info("progress", zap.Int("numSpans", w.spanCount))

	if w.config.MaxSpansCount > 0 && w.spanCount >= w.config.MaxSpansCount {
		w.logger.Info("Saved enough spans, exiting...")
		w.closeLocked()
		return ErrMaxSpansCountReached
	}

	return nil
}

// truncate keeps the first maxSpans spans of the traces, and drops the scope and resource entries
// left without spans.
func truncate(traces ptrace.Traces, maxSpans int) {
	kept := 0
	traces.ResourceSpans().RemoveIf(func(rs ptrace.ResourceSpans) bool {
		rs.ScopeSpans().RemoveIf(func(ss ptrace.ScopeSpans) bool {
			ss.Spans().RemoveIf(func(ptrace.Span) bool {
				kept++
				return kept > maxSpans
			})
			return ss.Spans().Len() == 0
		})
		return rs.ScopeSpans().Len() == 0
	})
}

// Close closes the captured and anonymized files. It is safe to call multiple times.
func (w *Writer) Close() {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.closeLocked()
}

func (w *Writer) closeLocked() {
	if w.closed {
		return
	}
	w.closed = true

	if w.capturedFile != nil {
		w.flushTraces(w.capturedFile, w.captured)
		w.capturedFile.Close()
	}
	if w.anonymizedFile != nil {
		w.flushTraces(w.anonymizedFile, w.anonymized)
		w.anonymizedFile.Close()
	}
	if w.anonymizer != nil {
		w.anonymizer.Stop()
		w.anonymizer.SaveMapping()
	}
}

func (w *Writer) flushTraces(file *os.File, traces ptrace.Traces) {
	var marshaler ptrace.JSONMarshaler
	dat, err := marshaler.MarshalTraces(traces)
	if err != nil {
		w.logger.Error("cannot marshal traces", zap.Error(err))
		return
	}
	if _, err := file.Write(dat); err != nil {
		w.logger.Error("cannot write output file", zap.String("file", file.Name()), zap.Error(err))
	}
}

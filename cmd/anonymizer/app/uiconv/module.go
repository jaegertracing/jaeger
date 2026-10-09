// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"fmt"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"go.uber.org/zap"
)

// Config for the extractor.
type Config struct {
	CapturedFile string `yaml:"captured_file"`
	UIFile       string `yaml:"ui_file"`
	TraceID      string `yaml:"trace_id"`
}

// Extract reads anonymized file, finds spans for a given trace,
// and writes out that trace in the UI format.
func Extract(config Config, logger *zap.Logger) error {
	traceID, err := model.TraceIDFromString(config.TraceID)
	if err != nil {
		return fmt.Errorf("cannot parse trace ID: %w", err)
	}
	reader, err := newSpanReader(config.CapturedFile, logger)
	if err != nil {
		return err
	}
	ext, err := newExtractor(config.UIFile, fmt.Sprintf("%016x%016x", traceID.High, traceID.Low), reader, logger)
	if err != nil {
		return err
	}
	return ext.Run()
}

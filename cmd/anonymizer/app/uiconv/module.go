// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
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
	reader, err := newSpanReader(config.CapturedFile, logger)
	if err != nil {
		return err
	}
	ext, err := newExtractor(config.UIFile, config.TraceID, reader, logger)
	if err != nil {
		return err
	}
	return ext.Run()
}

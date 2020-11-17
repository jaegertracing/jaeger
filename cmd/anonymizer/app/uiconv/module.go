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
	reader, err := NewReader(config.CapturedFile, logger)
	if err != nil {
		return err
	}
	ext, err := NewExtractor(config.UIFile, config.TraceID, reader, logger)
	if err != nil {
		return err
	}
	return ext.Run()
}

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

package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/anonymizer"
	"github.com/jaegertracing/jaeger/model"
)

// Config contains parameters to NewWriter.
type Config struct {
	MaxSpansCount  int                `yaml:"max_spans_count" name:"max_spans_count"`
	CapturedFile   string             `yaml:"captured_file" name:"captured_file"`
	AnonymizedFile string             `yaml:"anonymized_file" name:"anonymized_file"`
	MappingFile    string             `yaml:"mapping_file" name:"mapping_file"`
	AnonymizerOpts anonymizer.Options `yaml:"anonymizer" name:"anonymizer"`
}

// Writer is a span Writer that obfuscates the span and writes it to a JSON file.
type Writer struct {
	config         Config
	lock           sync.Mutex
	logger         *zap.Logger
	capturedFile   *os.File
	anonymizedFile *os.File
	anonymizer     *anonymizer.Anonymizer
	spanCount      int
}

// New creates an Writer
func New(config Config, logger *zap.Logger) (*Writer, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	logger.Sugar().Infof("Current working dir is %s", wd)

	cf, err := os.OpenFile(config.CapturedFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing captured spans to file %s", config.CapturedFile)

	af, err := os.OpenFile(config.AnonymizedFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create output file: %w", err)
	}
	logger.Sugar().Infof("Writing anonymized spans to file %s", config.AnonymizedFile)

	_, err = cf.WriteString("[")
	if err != nil {
		return nil, fmt.Errorf("cannot write tp output file: %w", err)
	}
	_, err = af.WriteString("[")
	if err != nil {
		return nil, fmt.Errorf("cannot write tp output file: %w", err)
	}

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
	}, nil
}

// WriteSpan anonymized the span and appends it as JSON to w.file.
func (w *Writer) WriteSpan(msg *model.Span) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	out := new(bytes.Buffer)
	if err := new(jsonpb.Marshaler).Marshal(out, msg); err != nil {
		return err
	}
	if w.spanCount > 0 {
		w.capturedFile.WriteString(",\n")
	}
	w.capturedFile.Write(out.Bytes())
	w.capturedFile.Sync()

	span := w.anonymizer.AnonymizeSpan(msg)

	dat, err := json.Marshal(span)
	if err != nil {
		return err
	}
	if w.spanCount > 0 {
		w.anonymizedFile.WriteString(",\n")
	}
	if _, err := w.anonymizedFile.Write(dat); err != nil {
		return err
	}
	w.anonymizedFile.Sync()

	w.spanCount++
	if w.spanCount%100 == 0 {
		w.logger.Info("progress", zap.Int("numSpans", w.spanCount))
	}

	if w.config.MaxSpansCount > 0 && w.spanCount >= w.config.MaxSpansCount {
		w.logger.Info("Saved enough spans, exiting...")
		w.Close()
		os.Exit(0)
	}

	return nil
}

// Close closes the captured and anonymized files.
func (w *Writer) Close() {
	w.capturedFile.WriteString("\n]\n")
	w.capturedFile.Close()
	w.anonymizedFile.WriteString("\n]\n")
	w.anonymizedFile.Close()
	w.anonymizer.SaveMapping()
}

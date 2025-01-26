// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
		return ErrMaxSpansCountReached
	}

	return nil
}

// Close closes the captured and anonymized files.
func (w *Writer) Close() {
	w.capturedFile.WriteString("\n]\n")
	w.capturedFile.Close()
	w.anonymizedFile.WriteString("\n]\n")
	w.anonymizedFile.Close()
	w.anonymizer.Stop()
	w.anonymizer.SaveMapping()
}

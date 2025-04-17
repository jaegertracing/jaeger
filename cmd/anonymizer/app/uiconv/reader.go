// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/uimodel"
)

var errNoMoreSpans = errors.New("no more spans")

// spanReader loads previously captured spans from a file.
type spanReader struct {
	logger       *zap.Logger
	capturedFile *os.File
	reader       *bufio.Reader
	spansRead    int
	eofReached   bool
}

// newSpanReader creates a spanReader.
func newSpanReader(capturedFile string, logger *zap.Logger) (*spanReader, error) {
	cf, err := os.OpenFile(capturedFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot open captured file: %w", err)
	}
	logger.Sugar().Infof("Reading captured spans from file %s", capturedFile)

	return &spanReader{
		logger:       logger,
		capturedFile: cf,
		reader:       bufio.NewReader(cf),
	}, nil
}

// NextSpan reads the next span from the capture file, or returns errNoMoreSpans.
func (r *spanReader) NextSpan() (*uimodel.Span, error) {
	if r.eofReached {
		return nil, errNoMoreSpans
	}
	if r.spansRead == 0 {
		b, err := r.reader.ReadByte()
		if err != nil {
			r.eofReached = true
			return nil, fmt.Errorf("cannot read file: %w", err)
		}
		if b != '[' {
			r.eofReached = true
			return nil, errors.New("file must begin with '['")
		}
	}
	s, err := r.reader.ReadString('\n')
	if err != nil {
		r.eofReached = true
		return nil, fmt.Errorf("cannot read file: %w", err)
	}
	if s[len(s)-2] == ',' { // all but last span lines end with ,\n
		s = s[0 : len(s)-2]
	} else {
		r.eofReached = true
	}
	var span uimodel.Span
	err = json.Unmarshal([]byte(s), &span)
	if err != nil {
		r.eofReached = true
		return nil, fmt.Errorf("cannot unmarshal span: %w; %s", err, s)
	}
	r.spansRead++
	if r.spansRead%1000 == 0 {
		r.logger.Info("Scan progress", zap.Int("span_count", r.spansRead))
	}
	return &span, nil
}

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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	uimodel "github.com/jaegertracing/jaeger/model/json"
)

// Reader loads previously captured spans from a file.
type Reader struct {
	logger       *zap.Logger
	capturedFile *os.File
	reader       *bufio.Reader
	spansRead    int
	eofReached   bool
}

// NewReader creates a Reader.
func NewReader(capturedFile string, logger *zap.Logger) (*Reader, error) {
	cf, err := os.OpenFile(capturedFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot open captured file: %w", err)
	}
	logger.Sugar().Infof("Reading captured spans from file %s", capturedFile)

	return &Reader{
		logger:       logger,
		capturedFile: cf,
		reader:       bufio.NewReader(cf),
	}, nil
}

// NextSpan reads the next span from the capture file, or returns io.EOF.
func (r *Reader) NextSpan() (*uimodel.Span, error) {
	if r.eofReached {
		return nil, io.EOF
	}
	if r.spansRead == 0 {
		b, err := r.reader.ReadByte()
		if err != nil {
			r.eofReached = true
			return nil, fmt.Errorf("cannot read file: %w", err)
		}
		if b != '[' {
			r.eofReached = true
			return nil, fmt.Errorf("file must begin with '['")
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

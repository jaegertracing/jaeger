// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// OutputFormat defines format types
type OutputFormat string

const (
	FormatJSON       OutputFormat = "json"
	FormatJSONPretty OutputFormat = "json-pretty"
)

// Printer handles output formatting
type Printer struct {
	format OutputFormat
	writer io.Writer
}

// NewPrinter creates a printer
func NewPrinter(format OutputFormat, writer io.Writer) *Printer {
	if writer == nil {
		writer = os.Stdout
	}
	return &Printer{
		format: format,
		writer: writer,
	}
}

// Print outputs the collection
func (p *Printer) Print(collection *ConfigCollection) error {
	var output []byte
	var err error

	if p.format == FormatJSONPretty {
		output, err = json.MarshalIndent(collection, "", "  ")
	} else {
		output, err = json.Marshal(collection)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, err = p.writer.Write(output)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	_, err = p.writer.Write([]byte("\n"))
	return err
}

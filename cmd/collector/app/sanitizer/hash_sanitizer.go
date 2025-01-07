// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"bytes"
	"encoding/hex"

	"github.com/jaegertracing/jaeger/model"
)

// NewHashingSanitizer creates a sanitizer to add hash field to spans
func NewHashingSanitizer() SanitizeSpan {
	return hashingSanitizer
}

func hashingSanitizer(span *model.Span) *model.Span {
	// Check if hash already exists
	for _, tag := range span.Tags {
		if tag.Key == "span.hash" {
			return span
		}
	}

	buf := &bytes.Buffer{}
	if err := span.Hash(buf); err != nil {
		return span
	}

	hashStr := hex.EncodeToString(buf.Bytes())
	span.Tags = append(span.Tags, model.KeyValue{
		Key:   "span.hash",
		VType: model.ValueType_STRING,
		VStr:  hashStr,
	})

	return span
}

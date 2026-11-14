// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHoistedColumns(t *testing.T) {
	validAttribute := HoistedAttribute{
		Key:    "gen_ai.request.model",
		Level:  "span",
		Type:   "string",
		Column: "gen_ai_request_model",
	}
	validWith := func(mutate func(attribute *HoistedAttribute)) []HoistedAttribute {
		attribute := validAttribute
		mutate(&attribute)
		return []HoistedAttribute{attribute}
	}
	tests := []struct {
		name       string
		attributes []HoistedAttribute
		wantErr    string
	}{
		{
			name:       "one entry",
			attributes: []HoistedAttribute{validAttribute},
		},
		{
			name: "same key at two types and two levels",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    validAttribute.Key,
					Level:  "span",
					Type:   "int",
					Column: "model_int",
				},
				{
					Key:    validAttribute.Key,
					Level:  "resource",
					Type:   "string",
					Column: "resource_model",
					Codec:  "ZSTD(22)",
				},
			},
		},
		{
			name:       "empty key",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Key = "" }),
			wantErr:    "hoisted_attributes[0]: key must not be empty",
		},
		{
			name:       "unsupported level",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Level = "scope" }),
			wantErr:    `level must be span or resource, not "scope"`,
		},
		{
			name:       "unsupported type",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Type = "bytes" }),
			wantErr:    `type must be string, int, double or bool, not "bytes"`,
		},
		{
			name:       "column is not an identifier",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "model; DROP TABLE spans" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "column that starts like a virtual column",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "_row_exists" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "empty column",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "column of the spans table",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "trace_id" }),
			wantErr:    `column "trace_id" is already a column`,
		},
		{
			name:       "attribute group of the spans table",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "resource_str_attributes" }),
			wantErr:    `column "resource_str_attributes" is already a column`,
		},
		{
			name: "column used twice",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    "other",
					Level:  "span",
					Type:   "string",
					Column: validAttribute.Column,
				},
			},
			wantErr: `hoisted_attributes[1]: column "gen_ai_request_model" is already a column`,
		},
		{
			name: "attribute listed twice",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    validAttribute.Key,
					Level:  validAttribute.Level,
					Type:   validAttribute.Type,
					Column: "second_copy",
				},
			},
			wantErr: `the span attribute "gen_ai.request.model" of type string is listed twice`,
		},
		{
			name:       "codec other than ZSTD",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "LZ4" }),
			wantErr:    `codec "LZ4" must be ZSTD(level)`,
		},
		{
			name:       "codec with SQL after it",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(1)) TTL now(" }),
			wantErr:    "must be ZSTD(level)",
		},
		{
			name:       "codec level zero",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(0)" }),
			wantErr:    "must be ZSTD(level)",
		},
		{
			name:       "codec level above the maximum",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(23)" }),
			wantErr:    "must be ZSTD(level)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHoistedColumns(tt.attributes)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

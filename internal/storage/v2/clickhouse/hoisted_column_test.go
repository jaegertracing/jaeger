// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_HoistedAttributes(t *testing.T) {
	valid := HoistedAttribute{
		Key:    "gen_ai.request.model",
		Level:  "span",
		Type:   "string",
		Column: "gen_ai_request_model",
	}
	with := func(mutate func(h *HoistedAttribute)) []HoistedAttribute {
		h := valid
		mutate(&h)
		return []HoistedAttribute{h}
	}
	tests := []struct {
		name    string
		hoisted []HoistedAttribute
		wantErr string
	}{
		{
			name:    "one entry",
			hoisted: []HoistedAttribute{valid},
		},
		{
			name: "same key at two types and two levels",
			hoisted: []HoistedAttribute{
				valid,
				{
					Key:    valid.Key,
					Level:  "span",
					Type:   "int",
					Column: "model_int",
				},
				{
					Key:    valid.Key,
					Level:  "resource",
					Type:   "string",
					Column: "resource_model",
					Codec:  "ZSTD(22)",
				},
			},
		},
		{
			name:    "empty key",
			hoisted: with(func(h *HoistedAttribute) { h.Key = "" }),
			wantErr: "hoisted_attributes[0]: key must not be empty",
		},
		{
			name:    "unsupported level",
			hoisted: with(func(h *HoistedAttribute) { h.Level = "scope" }),
			wantErr: `level must be span or resource, not "scope"`,
		},
		{
			name:    "unsupported type",
			hoisted: with(func(h *HoistedAttribute) { h.Type = "bytes" }),
			wantErr: `type must be string, int, double or bool, not "bytes"`,
		},
		{
			name:    "column is not an identifier",
			hoisted: with(func(h *HoistedAttribute) { h.Column = "model; DROP TABLE spans" }),
			wantErr: "must match [a-z_][a-z0-9_]*",
		},
		{
			name:    "empty column",
			hoisted: with(func(h *HoistedAttribute) { h.Column = "" }),
			wantErr: "must match [a-z_][a-z0-9_]*",
		},
		{
			name:    "column of the spans table",
			hoisted: with(func(h *HoistedAttribute) { h.Column = "trace_id" }),
			wantErr: `column "trace_id" is already a column`,
		},
		{
			name:    "attribute group of the spans table",
			hoisted: with(func(h *HoistedAttribute) { h.Column = "resource_str_attributes" }),
			wantErr: `column "resource_str_attributes" is already a column`,
		},
		{
			name: "column used twice",
			hoisted: []HoistedAttribute{
				valid,
				{
					Key:    "other",
					Level:  "span",
					Type:   "string",
					Column: valid.Column,
				},
			},
			wantErr: `hoisted_attributes[1]: column "gen_ai_request_model" is already a column`,
		},
		{
			name: "attribute listed twice",
			hoisted: []HoistedAttribute{
				valid,
				{
					Key:    valid.Key,
					Level:  valid.Level,
					Type:   valid.Type,
					Column: "second_copy",
				},
			},
			wantErr: `the span attribute "gen_ai.request.model" of type string is listed twice`,
		},
		{
			name:    "codec other than ZSTD",
			hoisted: with(func(h *HoistedAttribute) { h.Codec = "LZ4" }),
			wantErr: `codec "LZ4" must be ZSTD(level)`,
		},
		{
			name:    "codec with SQL after it",
			hoisted: with(func(h *HoistedAttribute) { h.Codec = "ZSTD(1)) TTL now(" }),
			wantErr: "must be ZSTD(level)",
		},
		{
			name:    "codec level zero",
			hoisted: with(func(h *HoistedAttribute) { h.Codec = "ZSTD(0)" }),
			wantErr: "must be ZSTD(level)",
		},
		{
			name:    "codec level above the maximum",
			hoisted: with(func(h *HoistedAttribute) { h.Codec = "ZSTD(23)" }),
			wantErr: "must be ZSTD(level)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfiguration()
			cfg.Addresses = []string{"localhost:9000"}
			cfg.HoistedAttributes = tt.hoisted

			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

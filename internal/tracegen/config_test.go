// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func Test_Run(t *testing.T) {
	logger := zap.NewNop()
	tp := sdktrace.NewTracerProvider()

	tests := []struct {
		name        string
		config      *Config
		expectedErr error
	}{
		{
			name:        "Empty config",
			config:      &Config{},
			expectedErr: errors.New("either `traces` or `duration` must be greater than 0"),
		},
		{
			name: "Non-empty config",
			config: &Config{
				Workers:    2,
				Traces:     10,
				ChildSpans: 5,
				Attributes: 20,
				AttrKeys:   50,
				AttrValues: 100,
			},
			expectedErr: nil,
		},
		{
			name: "Negative traces, positive duration",
			config: &Config{
				Traces:   -7,
				Duration: 7,
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracers := []trace.Tracer{tp.Tracer("Test-Tracer")}
			err := Run(tt.config, tracers, logger)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func Test_Flags(t *testing.T) {
	fs := &flag.FlagSet{}
	config := &Config{}
	expectedConfig := &Config{
		Workers:             1,
		Traces:              1,
		ChildSpans:          1,
		Attributes:          11,
		AttrKeys:            97,
		AttrValues:          1000,
		Debug:               false,
		Firehose:            false,
		Pause:               1000,
		Duration:            0,
		Service:             "tracegen",
		Services:            1,
		TraceExporter:       "otlp-http",
		Repeatable:          false,
		RepeatableTimeStart: "2025-01-01T00:00:00.000000000+00:00",
	}

	config.Flags(fs)
	assert.Equal(t, expectedConfig, config)
}

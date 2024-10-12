// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestEmptyServiceNameSanitizer_SubstitutesCorrectlyForStrings(t *testing.T) {
	emptyServiceName := ""
	nonEmptyServiceName := "hello"
	tests := []struct {
		name                string
		serviceName         *string
		expectedServiceName string
	}{
		{
			name:                "no service name",
			expectedServiceName: "missing-service-name",
		},
		{
			name:                "empty service name",
			serviceName:         &emptyServiceName,
			expectedServiceName: "empty-service-name",
		},
		{
			name:                "non-empty service name",
			serviceName:         &nonEmptyServiceName,
			expectedServiceName: "hello",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			attributes := traces.
				ResourceSpans().
				AppendEmpty().
				Resource().
				Attributes()
			if test.serviceName != nil {
				attributes.PutStr("service.name", *test.serviceName)
			}
			sanitizer := NewEmptyServiceNameSanitizer()
			sanitized := sanitizer(traces)
			serviceName, ok := sanitized.
				ResourceSpans().
				At(0).
				Resource().
				Attributes().
				Get("service.name")
			require.True(t, ok)
			require.Equal(t, test.expectedServiceName, serviceName.Str())
		})
	}
}

func TestEmptyServiceNameSanitizer_SubstitutesCorrectlyForNonStringType(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes().
		PutInt("service.name", 1)
	sanitizer := NewEmptyServiceNameSanitizer()
	sanitized := sanitizer(traces)
	serviceName, ok := sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get("service.name")
	require.True(t, ok)
	require.Equal(t, "service-name-wrong-type", serviceName.Str())
}

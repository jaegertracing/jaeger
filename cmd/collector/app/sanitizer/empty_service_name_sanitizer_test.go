// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestEmptyServiceNameSanitizer(t *testing.T) {
	s := NewEmptyServiceNameSanitizer()
	s1 := s(&model.Span{})
	assert.NotNil(t, s1.Process)
	assert.Equal(t, nullProcessServiceName, s1.Process.ServiceName)
	s2 := s(&model.Span{Process: &model.Process{}})
	assert.Equal(t, serviceNameReplacement, s2.Process.ServiceName)
}

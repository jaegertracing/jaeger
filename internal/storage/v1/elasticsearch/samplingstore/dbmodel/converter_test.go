// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestConvertDependencies(t *testing.T) {
	tests := []struct {
		throughputs []*model.Throughput
	}{
		{
			throughputs: []*model.Throughput{{Service: "service1", Operation: "operation1", Count: 10, Probabilities: map[string]struct{}{"new-srv": {}}}},
		},
		{
			throughputs: []*model.Throughput{},
		},
		{
			throughputs: nil,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := FromThroughputs(test.throughputs)
			a := ToThroughputs(got)
			assert.Equal(t, test.throughputs, a)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

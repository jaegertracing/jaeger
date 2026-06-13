// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
)

func FromThroughputs(throughputs []*model.Throughput) []Throughput {
	if throughputs == nil {
		return nil
	}
	ret := make([]Throughput, len(throughputs))
	for i, d := range throughputs {
		ret[i] = Throughput{
			Service:       d.Service,
			Operation:     d.Operation,
			Count:         d.Count,
			Probabilities: d.Probabilities,
		}
	}
	return ret
}

func ToThroughputs(throughputs []Throughput) []*model.Throughput {
	if throughputs == nil {
		return nil
	}
	ret := make([]*model.Throughput, len(throughputs))
	for i, d := range throughputs {
		ret[i] = &model.Throughput{
			Service:       d.Service,
			Operation:     d.Operation,
			Count:         d.Count,
			Probabilities: d.Probabilities,
		}
	}
	return ret
}

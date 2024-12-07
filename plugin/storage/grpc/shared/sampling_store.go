// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"strconv"

	samplingStoreModel "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

func stringSetTofloat64Array(asSet map[string]struct{}) ([]float64, error) {
	asArray := []float64{}
	for k := range asSet {
		floatK, err := strconv.ParseFloat(k, 64)
		if err != nil {
			return nil, err
		}
		asArray = append(asArray, floatK)
	}
	return asArray, nil
}

func samplingStoreThroughputsToStorageV1Throughputs(throughputs []*samplingStoreModel.Throughput) ([]*storage_v1.Throughput, error) {
	storageV1Throughput := []*storage_v1.Throughput{}
	for _, throughput := range throughputs {
		probsAsArray, err := stringSetTofloat64Array(throughput.Probabilities)
		if err != nil {
			return nil, err
		}

		storageV1Throughput = append(storageV1Throughput, &storage_v1.Throughput{
			Service:       throughput.Service,
			Operation:     throughput.Operation,
			Count:         throughput.Count,
			Probabilities: probsAsArray,
		})
	}

	return storageV1Throughput, nil
}

func float64ArrayToStringSet(asArray []float64) map[string]struct{} {
	asSet := make(map[string]struct{})
	for _, prob := range asArray {
		asSet[strconv.FormatFloat(prob, 'E', -1, 64)] = struct{}{}
	}
	return asSet
}

func storageV1ThroughputsToSamplingStoreThroughputs(thoughputs []*storage_v1.Throughput) []*samplingStoreModel.Throughput {
	resThroughput := []*samplingStoreModel.Throughput{}

	for _, throughput := range thoughputs {
		resThroughput = append(resThroughput, &samplingStoreModel.Throughput{
			Service:       throughput.Service,
			Operation:     throughput.Operation,
			Count:         throughput.Count,
			Probabilities: float64ArrayToStringSet(throughput.Probabilities),
		})
	}

	return resThroughput
}

func stringFloatMapToV1StringFloatMap(in map[string]float64) *storage_v1.StringFloatMap {
	return &storage_v1.StringFloatMap{
		StringFloatMap: in,
	}
}

func sSFloatMapToStorageV1SSFloatMap(in map[string]map[string]float64) map[string]*storage_v1.StringFloatMap {
	res := make(map[string]*storage_v1.StringFloatMap)
	for k, v := range in {
		res[k] = stringFloatMapToV1StringFloatMap(v)
	}
	return res
}

func storageV1StringFloatMapToStringFloatMap(in *storage_v1.StringFloatMap) map[string]float64 {
	return in.StringFloatMap
}

func storageV1SSFloatMapToSSFloatMap(in map[string]*storage_v1.StringFloatMap) map[string]map[string]float64 {
	res := make(map[string]map[string]float64)
	for k, v := range in {
		res[k] = storageV1StringFloatMapToStringFloatMap(v)
	}
	return res
}

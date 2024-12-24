// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package iter is a backport of Go 1.23 official "iter" package, until we upgrade.
package iter

type (
	Seq[V any]     func(yield func(V) bool)
	Seq2[K, V any] func(yield func(K, V) bool)
)

func CollectWithErrors[V any](seq Seq2[V, error]) ([]V, error) {
	var result []V
	var err error
	seq(func(v V, e error) bool {
		if e != nil {
			err = e
			return false
		}
		result = append(result, v)
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func FlattenWithErrors[V any](seq Seq2[[]V, error]) ([]V, error) {
	var result []V
	var err error
	seq(func(v []V, e error) bool {
		if e != nil {
			err = e
			return false
		}
		result = append(result, v...)
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

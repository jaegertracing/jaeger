// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package badger

func (*Factory) diskStatisticsUpdate() error {
	return nil
}

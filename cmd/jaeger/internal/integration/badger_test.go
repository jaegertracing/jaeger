// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
)

func TestBadgerStorage(t *testing.T) {
	s := &StorageIntegration{
		Name:       "badger",
		ConfigFile: "fixtures/badger_config.yaml",
	}
	s.Test(t)
}

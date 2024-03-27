// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
)

func TestElasticsearchStorage(t *testing.T) {
	s := &StorageIntegration{
		Name:       "elasticsearch",
		ConfigFile: "fixtures/elasticsearch_config.yaml",
	}
	s.Test(t)
}

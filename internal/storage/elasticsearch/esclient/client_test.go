// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestClientWithVersion(t *testing.T) {
	// version is unexported and has no accessor (it stays encapsulated); this
	// in-package test reads the field directly to check WithVersion's behavior.
	base := Client{}
	assert.Equal(t, es.BackendVersion(0), base.version, "version defaults to unset")

	versioned := base.WithVersion(es.OpenSearch2)
	assert.Equal(t, es.OpenSearch2, versioned.version)
	assert.Equal(t, es.BackendVersion(0), base.version, "WithVersion must not mutate the receiver")
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package healthcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestStatusString(t *testing.T) {
	tests := map[Status]string{
		Unavailable: "unavailable",
		Ready:       "ready",
		Broken:      "broken",
		Status(-1):  "unknown",
	}
	for k, v := range tests {
		assert.Equal(t, v, k.String())
	}
}

func TestStatusSetGet(t *testing.T) {
	hc := New()
	assert.Equal(t, Unavailable, hc.Get())

	logger, logBuf := testutils.NewLogger()
	hc = New()
	hc.SetLogger(logger)
	assert.Equal(t, Unavailable, hc.Get())

	hc.Ready()
	assert.Equal(t, Ready, hc.Get())
	assert.Equal(t, map[string]string{"level": "info", "msg": "Health Check state change", "status": "ready"}, logBuf.JSONLine(0))
}

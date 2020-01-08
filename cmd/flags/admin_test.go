// Copyright (c) 2020 The Jaeger Authors.
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

package flags

import (
	"context"
	"sync"
	"testing"

	"github.com/marusama/cyclicbarrier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestAdminServerHandlesPortZero(t *testing.T) {
	adminServer := NewAdminServer(0)

	v, _ := config.Viperize(adminServer.AddFlags)

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	adminServer.initFromViper(v, logger)

	assert.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	message := logs.FilterMessage("Admin server started")
	assert.Equal(t, 1, message.Len(), "Expected Admin server started log message.")

	onlyEntry := message.All()[0]
	port := onlyEntry.ContextMap()["http-port"].(int64)
	assert.Greater(t, port, int64(0))
}

func TestAdminServerHandlesSimultaneousStartupOnPortZero(t *testing.T) {
	const parallelInstances = 2

	var wg sync.WaitGroup
	barrier := cyclicbarrier.New(parallelInstances)

	for i := 0; i < parallelInstances; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			adminServer := NewAdminServer(0)

			v, _ := config.Viperize(adminServer.AddFlags)
			logger := zap.NewNop()
			adminServer.initFromViper(v, logger)

			if assert.NoError(t, adminServer.Serve(), "Failed to start two Admin servers at port 0") {
				defer adminServer.Close()
			}

			require.NoError(t, barrier.Await(context.Background()))
		}()
	}

	wg.Wait()
}

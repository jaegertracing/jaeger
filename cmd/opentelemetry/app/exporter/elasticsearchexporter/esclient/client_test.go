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

package esclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

func TestGetClient(t *testing.T) {
	tests := []struct {
		version int
		err     string
	}{
		{
			version: 5,
		},
		{
			version: 6,
		},
		{
			version: 7,
		},
		{
			version: 1,
			err:     "could not create Elasticseach client for version 1",
		},
	}
	for _, test := range tests {
		t.Run(test.err, func(t *testing.T) {
			client, err := NewElasticsearchClient(config.Configuration{Servers: []string{""}, Version: uint(test.version)}, zap.NewNop())
			if test.err == "" {
				require.NoError(t, err)
			}
			switch test.version {
			case 5, 6:
				assert.IsType(t, &elasticsearch6Client{}, client)
			case 7:
				assert.IsType(t, &elasticsearch7Client{}, client)
			default:
				assert.EqualError(t, err, test.err)
				assert.Nil(t, client)
			}
		})
	}
}

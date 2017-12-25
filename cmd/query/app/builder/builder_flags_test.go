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

package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestQueryBuilderFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.static-files=/dev/null",
		"--query.ui-config=some.json",
		"--query.prefix=api",
		"--query.port=80",
	})
	qOpts := new(QueryOptions).InitFromViper(v)
	assert.Equal(t, "/dev/null", qOpts.StaticAssets)
	assert.Equal(t, "some.json", qOpts.UIConfig)
	assert.Equal(t, "api", qOpts.Prefix)
	assert.Equal(t, 80, qOpts.Port)
}

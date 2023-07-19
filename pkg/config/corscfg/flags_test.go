// Copyright (c) 2023 The Jaeger Authors.
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

package corscfg

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestCORSFlags(t *testing.T) {
	cmdFlags := []string{
		"--prefix.cors.allowed-headers=Content-Type, Accept, X-Requested-With",
		"--prefix.cors.allowed-origins=http://example.domain.com, http://*.domain.com",
	}
	t.Run("CORS Flags", func(t *testing.T) {
		flagCfg := Flags{
			Prefix: "prefix",
		}
		v, command := config.Viperize(flagCfg.AddFlags)

		err := command.ParseFlags(cmdFlags)
		require.NoError(t, err)

		corsOpts := flagCfg.InitFromViper(v)
		fmt.Println(corsOpts)

		assert.Equal(t, Options{
			AllowedHeaders: []string{"Content-Type", "Accept", "X-Requested-With"},
			AllowedOrigins: []string{"http://example.domain.com", "http://*.domain.com"},
		}, corsOpts)
	})
}

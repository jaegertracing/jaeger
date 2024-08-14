// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package corscfg

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

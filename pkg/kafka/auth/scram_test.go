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

package auth_test

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

func TestSCRAMClientFlags(t *testing.T) {

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.String("--kafka.scram.username", "fakeuser", "")
		flagSet.String("--kafka.scram.password", "fakepassword", "")
		flagSet.String("--kafka.scram.algorithm", "sha256", "")
	}

	v, _ := config.Viperize(addFlags)

	authCfg := auth.AuthenticationConfig{
		Authentication: "scram",
	}

	authCfg.InitFromViper("--kafka", v)
	// check to see if the configs are the same
	assert.Equal(t, auth.ScramConfig{
		UserName:  "fakeuser",
		Password:  "fakepassword",
		Algorithm: "sha256",
	}, authCfg.SCRAM)
}

// testing Begin, Step, and Done require a network connection to test,
// they otherwise will have a perpetual connection that cannot be closed

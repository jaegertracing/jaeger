// Copyright (c) 2018 The Jaeger Authors.
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

package reporter_test

import (
	"flag"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
)

func TestBindFlags_NoJaegerTags(t *testing.T) {
	v := viper.New()
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	reporter.AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--reporter.type=grpc",
	})
	require.NoError(t, err)

	b := &reporter.Options{}
	b.InitFromViper(v, zap.NewNop())
	assert.Equal(t, reporter.Type("grpc"), b.ReporterType)
	assert.Len(t, b.AgentTags, 0)
}

func TestBindFlags(t *testing.T) {
	v := viper.New()
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	reporter.AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	jaegerTags := fmt.Sprintf("%s,%s,%s,%s,%s,%s",
		"key=value",
		"envVar1=${envKey1:defaultVal1}",
		"envVar2=${envKey2:defaultVal2}",
		"envVar3=${envKey3}",
		"envVar4=${envKey4}",
		"envVar5=${envVar5:}",
	)

	err := command.ParseFlags([]string{
		"--reporter.type=grpc",
		"--jaeger.tags=" + jaegerTags,
	})
	require.NoError(t, err)

	b := &reporter.Options{}
	os.Setenv("envKey1", "envVal1")
	defer os.Unsetenv("envKey1")

	os.Setenv("envKey4", "envVal4")
	defer os.Unsetenv("envKey4")

	b.InitFromViper(v, zap.NewNop())

	expectedTags := map[string]string{
		"key":     "value",
		"envVar1": "envVal1",
		"envVar2": "defaultVal2",
		"envVar4": "envVal4",
		"envVar5": "",
	}

	assert.Equal(t, reporter.Type("grpc"), b.ReporterType)
	assert.Equal(t, expectedTags, b.AgentTags)
}

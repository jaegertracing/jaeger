// Copyright (c) 2018 Uber Technologies, Inc.
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

package strategystore

import (
	"errors"
	"flag"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin"
)

var _ ss.Factory = new(Factory)
var _ plugin.Configurable = new(Factory)

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(FactoryConfig{StrategyStoreType: staticStrategyStoreType})
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[staticStrategyStoreType])
	assert.Equal(t, staticStrategyStoreType, f.StrategyStoreType)

	mock := new(mockFactory)
	f.factories[staticStrategyStoreType] = mock

	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	_, err = f.CreateStrategyStore()
	assert.NoError(t, err)

	// force the mock to return errors
	mock.retError = true
	assert.Error(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	_, err = f.CreateStrategyStore()
	assert.Error(t, err)

	_, err = NewFactory(FactoryConfig{StrategyStoreType: "nonsense"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unknown sampling strategy store type")
}

func TestConfigurable(t *testing.T) {
	clearEnv()
	defer clearEnv()

	f, err := NewFactory(FactoryConfig{StrategyStoreType: staticStrategyStoreType})
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[staticStrategyStoreType])

	mock := new(mockFactory)
	f.factories[staticStrategyStoreType] = mock

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v)

	assert.Equal(t, fs, mock.flagSet)
	assert.Equal(t, v, mock.viper)
}

type mockFactory struct {
	flagSet  *flag.FlagSet
	viper    *viper.Viper
	retError bool
}

func (f *mockFactory) AddFlags(flagSet *flag.FlagSet) {
	f.flagSet = flagSet
}

func (f *mockFactory) InitFromViper(v *viper.Viper) {
	f.viper = v
}

func (f *mockFactory) CreateStrategyStore() (ss.StrategyStore, error) {
	if f.retError {
		return nil, errors.New("error")
	}
	return nil, nil
}

func (f *mockFactory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	if f.retError {
		return errors.New("error")
	}
	return nil
}

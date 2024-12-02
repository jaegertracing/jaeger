// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"flag"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/plugin/metricstore/disabled"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/mocks"
)

var _ storage.MetricStoreFactory = new(Factory)

func withConfig(storageType string) FactoryConfig {
	return FactoryConfig{
		MetricsStorageType: storageType,
	}
}

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(withConfig(prometheusStorageType))
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[prometheusStorageType])
	assert.Equal(t, prometheusStorageType, f.MetricsStorageType)
}

func TestUnsupportedMetricsStorageType(t *testing.T) {
	f, err := NewFactory(withConfig("foo"))
	require.Error(t, err)
	assert.Nil(t, f)
	require.EqualError(t, err, `unknown metrics type "foo". Valid types are [prometheus]`)
}

func TestDisabledMetricsStorageType(t *testing.T) {
	f, err := NewFactory(withConfig(disabledStorageType))
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.Equal(t, &disabled.Factory{}, f.factories[disabledStorageType])
	assert.Equal(t, disabledStorageType, f.MetricsStorageType)
}

func TestCreateMetricsReader(t *testing.T) {
	f, err := NewFactory(withConfig(prometheusStorageType))
	require.NoError(t, err)
	require.NotNil(t, f)

	require.NoError(t, f.Initialize(telemetry.NoopSettings()))

	reader, err := f.CreateMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)

	f.MetricsStorageType = "foo"
	reader, err = f.CreateMetricsReader()
	require.Error(t, err)
	require.Nil(t, reader)

	require.EqualError(t, err, `no "foo" backend registered for metrics store`)
}

type configurable struct {
	mocks.MetricsFactory
	flagSet *flag.FlagSet
	viper   *viper.Viper
	logger  *zap.Logger
}

// AddFlags implements plugin.Configurable.
func (f *configurable) AddFlags(flagSet *flag.FlagSet) {
	f.flagSet = flagSet
}

// InitFromViper implements plugin.Configurable.
func (f *configurable) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.viper = v
	f.logger = logger
}

func TestConfigurable(t *testing.T) {
	f, err := NewFactory(withConfig(prometheusStorageType))
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[prometheusStorageType])

	mock := new(configurable)
	f.factories[prometheusStorageType] = mock

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v, zap.NewNop())

	assert.Equal(t, fs, mock.flagSet)
	assert.Equal(t, v, mock.viper)
}

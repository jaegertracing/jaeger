// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"errors"
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

var (
	_ plugin.Configurable      = (*Factory)(nil)
	_ samplingstrategy.Factory = (*Factory)(nil)
)

// Factory implements samplingstrategy.Factory for an adaptive strategy store.
type Factory struct {
	options        *Options
	logger         *zap.Logger
	metricsFactory metrics.Factory
	lock           distributedlock.Lock
	store          samplingstore.Store
	participant    *leaderelection.DistributedElectionParticipant
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		options: &Options{},
		logger:  zap.NewNop(),
		lock:    nil,
		store:   nil,
	}
}

// AddFlags implements plugin.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.options.InitFromViper(v)
}

// Initialize implements samplingstrategy.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, ssFactory storage.SamplingStoreFactory, logger *zap.Logger) error {
	if ssFactory == nil {
		return errors.New("sampling store factory is nil. Please configure a backend that supports adaptive sampling")
	}
	var err error
	f.logger = logger
	f.metricsFactory = metricsFactory
	f.lock, err = ssFactory.CreateLock()
	if err != nil {
		return err
	}
	f.store, err = ssFactory.CreateSamplingStore(f.options.AggregationBuckets)
	if err != nil {
		return err
	}
	f.participant = leaderelection.NewElectionParticipant(f.lock, defaultResourceName, leaderelection.ElectionParticipantOptions{
		FollowerLeaseRefreshInterval: f.options.FollowerLeaseRefreshInterval,
		LeaderLeaseRefreshInterval:   f.options.LeaderLeaseRefreshInterval,
		Logger:                       f.logger,
	})
	f.participant.Start()

	return nil
}

// CreateStrategyProvider implements samplingstrategy.Factory
func (f *Factory) CreateStrategyProvider() (samplingstrategy.Provider, samplingstrategy.Aggregator, error) {
	s := NewProvider(*f.options, f.logger, f.participant, f.store)
	a, err := NewAggregator(*f.options, f.logger, f.metricsFactory, f.participant, f.store)
	if err != nil {
		return nil, nil, err
	}

	s.Start()
	a.Start()

	return s, a, nil
}

// Closes the factory
func (f *Factory) Close() error {
	return f.participant.Close()
}

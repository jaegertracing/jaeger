// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"errors"
	"reflect"
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
)

var (
	errNoSource       = errors.New("no sampling strategy specified, has to be either 'adaptive' or 'file'")
	errMultipleSource = errors.New("only one sampling strategy can be specified, has to be either 'adaptive' or 'file'")
)

type FileConfig struct {
	// File specifies a local file as the strategies source
	Path string `mapstructure:"path"`
}

type AdaptiveConfig struct {
	// name of the strategy storage defined in the jaegerstorage extension
	StrategyStore string `mapstructure:"strategy_store"`

	// InitialSamplingProbability is the initial sampling probability for all new operations.
	InitialSamplingProbability float64 `mapstructure:"initial_sampling_probability"`

	// AggregationBuckets is the total number of aggregated throughput buckets kept in memory, ie. if
	// the CalculationInterval is 1 minute (each bucket contains 1 minute of thoughput data) and the
	// AggregationBuckets is 3, the adaptive sampling processor will keep at most 3 buckets in memory for
	// all operations.
	// TODO(wjang): Expand on why this is needed when BucketsForCalculation seems to suffice.
	AggregationBuckets int `mapstructure:"aggregation_buckets"`

	// MinSamplesPerSecond determines the min number of traces that are sampled per second.
	// For example, if the value is 0.01666666666 (one every minute), then the sampling processor will do
	// its best to sample at least one trace a minute for an operation. This is useful for low QPS operations
	// that may never be sampled by the probabilistic sampler.
	MinSamplesPerSecond float64 `mapstructure:"min_samples_per_second"`

	// LeaderLeaseRefreshInterval is the duration to sleep if this processor is elected leader before
	// attempting to renew the lease on the leader lock. NB. This should be less than FollowerLeaseRefreshInterval
	// to reduce lock thrashing.
	LeaderLeaseRefreshInterval time.Duration `mapstructure:"leader_lease_refresh_interval"`

	// FollowerLeaseRefreshInterval is the duration to sleep if this processor is a follower
	// (ie. failed to gain the leader lock).
	FollowerLeaseRefreshInterval time.Duration `mapstructure:"follower_lease_refresh_interval"`
}

type Config struct {
	File     FileConfig     `mapstructure:"file"`
	Adaptive AdaptiveConfig `mapstructure:"adaptive"`
	HTTP     HTTPConfig     `mapstructure:"http"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
}

type HTTPConfig struct {
	confighttp.ServerConfig `mapstructure:",squash"`
}

type GRPCConfig struct {
	configgrpc.ServerConfig `mapstructure:",squash"`
}

func (cfg *Config) Validate() error {
	emptyCfg := createDefaultConfig().(*Config)
	if reflect.DeepEqual(*cfg, *emptyCfg) {
		return errNoSource
	}

	if cfg.File.Path != "" && cfg.Adaptive.StrategyStore != "" {
		return errMultipleSource
	}
	return nil
}

package adaptive

import (
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const (
	defaultAggregationInterval          = time.Minute
	defaultTargetQPS                    = 1
	defaultEquivalenceThreshold         = 0.3
	defaultLookbackQPSCount             = 1
	defaultCalculationInterval          = time.Minute
	defaultLookbackInterval             = time.Minute * 10
	defaultDelay                        = time.Minute * 2
	defaultSamplingProbability          = 0.001
	defaultMinSamplingProbability       = 0.00001                                      // once in 100 thousand requests
	defaultLowerBoundTracesPerSecond    = 1.0 / (1 * float64(time.Minute/time.Second)) // once every 1 minute
	defaultLeaderLeaseRefreshInterval   = 5 * time.Second
	defaultFollowerLeaseRefreshInterval = 60 * time.Second
)

// ThroughputAggregatorConfig is the configuration for the ThroughputAggregator.
type ThroughputAggregatorConfig struct {
	// AggregationInterval determines how often throughput is aggregated and written to storage.
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
}

// ProcessorConfig is the configuration for the SamplingProcessor.
type ProcessorConfig struct {
	// TargetQPS is the target sampled qps for all operations.
	TargetQPS float64 `yaml:"target_qps"`

	// QPSEquivalenceThreshold is the acceptable amount of deviation for the operation QPS from the `targetQPS`,
	// ie. [targetQPS-equivalenceThreshold, targetQPS+equivalenceThreshold] is the acceptable targetQPS range.
	// Increase this to reduce the amount of fluctuation in the probability calculation.
	QPSEquivalenceThreshold float64 `yaml:"qps_equivalence_threshold"`

	// LookbackQPSCount determines how many previous operation QPS are used in calculating the weighted QPS,
	// ie. if LookbackQPSCount is 1, the only the most recent QPS will be used in calculating the weighted QPS.
	LookbackQPSCount int `yaml:"lookback_qps_count"`

	// CalculationInterval determines the interval each bucket represents, ie. if an interval is
	// 1 minute, the bucket will contain 1 minute of throughput data for all services.
	CalculationInterval time.Duration `yaml:"calculation_interval"`

	// LookbackInterval is the total amount of throughput data used to calculate probabilities.
	LookbackInterval time.Duration `yaml:"lookback_interval"`

	// Delay is the amount of time to delay probability generation by, ie. if the calculationInterval
	// is 1 minute, the number of buckets is 10, and the delay is 2 minutes, then at one time
	// we'll have [now()-12,now()-2] range of throughput data in memory to base the calculations
	// off of.
	Delay time.Duration `yaml:"delay"`

	// DefaultSamplingProbability is the initial sampling probability for all new operations.
	DefaultSamplingProbability float64 `yaml:"default_sampling_probability"`

	// MinSamplingProbability is the minimum sampling probability for all operations. ie. the calculated sampling
	// probability will be bound [MinSamplingProbability, 1.0]
	MinSamplingProbability float64 `yaml:"min_sampling_probability"`

	// LowerBoundTracesPerSecond determines the lower bound number of traces that are sampled per second.
	// For example, if the value is 0.01666666666 (one every minute), then the sampling processor will do
	// its best to sample at least one trace a minute for an operation. This is useful for a low QPS operation
	// that is never sampled by the probabilistic sampler and depends on some time based element.
	LowerBoundTracesPerSecond float64 `yaml:"lower_bound_traces_per_second"`

	// LeaderLeaseRefreshInterval is the duration to sleep if this processor is elected leader before
	// attempting to renew the lease on the leader lock. NB. This should be less than FollowerLeaseRefreshInterval
	// to reduce lock thrashing.
	LeaderLeaseRefreshInterval time.Duration `yaml:"leader_lease_refresh_interval"`

	// FollowerLeaseRefreshInterval is the duration to sleep if this processor is a follower
	// (ie. failed to gain the leader lock).
	FollowerLeaseRefreshInterval time.Duration `yaml:"follower_lease_refresh_interval"`

	// Mutable is a configuration holder that holds configurations that could dynamically change during
	// the lifetime of the processor.
	Mutable MutableProcessorConfigurator
}

// MutableProcessorConfigurator is a mutable config holder for certain configs that can change during the lifetime
// of the processor.
type MutableProcessorConfigurator interface {
	GetTargetQPS() float64
	GetQPSEquivalenceThreshold() float64
}

// ImmutableProcessorConfig is a MutableProcessorConfigurator that doesn't dynamically update (it can be updated, but
// doesn't guarantee thread safety).
type ImmutableProcessorConfig struct {
	TargetQPS               float64 `json:"target_qps"`
	QPSEquivalenceThreshold float64 `json:"qps_equivalence_threshold"`
}

// GetTargetQPS implements MutableProcessorConfigurator#GetTargetQPS
func (d ImmutableProcessorConfig) GetTargetQPS() float64 {
	return d.TargetQPS
}

// GetQPSEquivalenceThreshold implements MutableProcessorConfigurator#GetQPSEquivalenceThreshold
func (d ImmutableProcessorConfig) GetQPSEquivalenceThreshold() float64 {
	return d.QPSEquivalenceThreshold
}

// MutableProcessorConfig is a MutableProcessorConfigurator that is thread safe and dynamically updates.
type MutableProcessorConfig struct {
	targetQPS               *atomic.Float64
	qpsEquivalenceThreshold *atomic.Float64
}

// NewMutableProcessorConfig returns a MutableProcessorConfigurator that dynamically updates.
func NewMutableProcessorConfig(config ImmutableProcessorConfig) *MutableProcessorConfig {
	return &MutableProcessorConfig{
		targetQPS:               atomic.NewFloat64(config.GetTargetQPS()),
		qpsEquivalenceThreshold: atomic.NewFloat64(config.GetQPSEquivalenceThreshold()),
	}
}

// Update updates the configs.
func (d *MutableProcessorConfig) Update(config ImmutableProcessorConfig) {
	d.targetQPS.Store(config.GetTargetQPS())
	d.qpsEquivalenceThreshold.Store(config.GetQPSEquivalenceThreshold())
}

// GetTargetQPS implements MutableProcessorConfigurator#GetTargetQPS.
func (d *MutableProcessorConfig) GetTargetQPS() float64 {
	return d.targetQPS.Load()
}

// GetQPSEquivalenceThreshold implements MutableProcessorConfigurator#GetQPSEquivalenceThreshold.
func (d *MutableProcessorConfig) GetQPSEquivalenceThreshold() float64 {
	return d.qpsEquivalenceThreshold.Load()
}

// Builder struct to hold configurations.
type Builder struct {
	ThroughputAggregator ThroughputAggregatorConfig `yaml:"throughput_aggregator"`
	SamplingProcessor    ProcessorConfig            `yaml:"sampling_processor"`

	metrics metrics.Factory
	logger  *zap.Logger
}

// NewBuilder creates a default builder.
func NewBuilder() *Builder {
	return &Builder{
		ThroughputAggregator: ThroughputAggregatorConfig{
			AggregationInterval: defaultAggregationInterval,
		},
		SamplingProcessor: ProcessorConfig{
			LookbackQPSCount:    defaultLookbackQPSCount,
			CalculationInterval: defaultCalculationInterval,
			LookbackInterval:    defaultLookbackInterval,
			Delay:               defaultDelay,
			DefaultSamplingProbability:   defaultSamplingProbability,
			MinSamplingProbability:       defaultMinSamplingProbability,
			LowerBoundTracesPerSecond:    defaultLowerBoundTracesPerSecond,
			LeaderLeaseRefreshInterval:   defaultLeaderLeaseRefreshInterval,
			FollowerLeaseRefreshInterval: defaultFollowerLeaseRefreshInterval,
			Mutable: ImmutableProcessorConfig{
				TargetQPS:               defaultTargetQPS,
				QPSEquivalenceThreshold: defaultEquivalenceThreshold,
			},
		},
	}
}

// WithMetricsFactory sets metrics factory.
func (b *Builder) WithMetricsFactory(m metrics.Factory) *Builder {
	b.metrics = m
	return b
}

// WithLogger sets logger.
func (b *Builder) WithLogger(l *zap.Logger) *Builder {
	b.logger = l
	return b
}

func (b *Builder) applyDefaults() {
	if b.metrics == nil {
		b.metrics = metrics.NullFactory
	}
	if b.logger == nil {
		b.logger = zap.NewNop()
	}
}

// NewThroughputAggregator creates and returns a ThroughputAggregator.
func (b *Builder) NewThroughputAggregator(storage samplingstore.Store) (Aggregator, error) {
	b.applyDefaults()
	return NewAggregator(b.metrics, b.ThroughputAggregator.AggregationInterval, storage), nil
}

// NewProcessor creates and returns a SamplingProcessor.
func (b *Builder) NewProcessor(hostname string, storage samplingstore.Store, lock distributedlock.Lock) (Processor, error) {
	b.applyDefaults()
	return NewProcessor(b.SamplingProcessor, hostname, storage, lock, b.metrics, b.logger)
}

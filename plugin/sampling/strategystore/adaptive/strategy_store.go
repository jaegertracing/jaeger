package adaptive

import (
	"os"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

// NewStrategyStore creates a strategy store that holds adaptive sampling strategies.
func NewStrategyStore(options Options, metricsFactory metrics.Factory, logger *zap.Logger, lock distributedlock.Lock, store samplingstore.Store) (*Processor, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	participant := leaderelection.NewElectionParticipant(lock, defaultResourceName, leaderelection.ElectionParticipantOptions{}) // todo(jpe) : wire up options/resource name
	p, err := newProcessor(options, hostname, store, participant, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	return p, nil
}

package http

import (
	"errors"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// SamplingManager returns sampling decisions from collector over HTTP.
type SamplingManager struct {
}

// NewConfigManager creates HTTP sampling manager.
func NewConfigManager(endpoint string) *SamplingManager {
	return &SamplingManager{}
}

// GetSamplingStrategy returns sampling strategies from collector.
func (s *SamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("sampling strategy not implemented")
}

// GetBaggageRestrictions returns baggage restrictions from collector.
func (s *SamplingManager) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	return nil, errors.New("baggage not implemented")
}

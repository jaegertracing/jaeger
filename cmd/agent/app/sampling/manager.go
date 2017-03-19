package sampling

import (
	"github.com/uber/jaeger/thrift-gen/sampling"
)

// Manager decides which sampling strategy a given service should be using.
type Manager interface {
	GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error)
}

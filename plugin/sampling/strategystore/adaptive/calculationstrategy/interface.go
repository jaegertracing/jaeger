package calculationstrategy

// ProbabilityCalculator calculates the new probability given the current and target QPS
type ProbabilityCalculator interface {
	Calculate(targetQPS, curQPS, prevProbability float64) (newProbability float64)
}

// Func wraps a function of appropriate signature and makes a ProbabilityCalculator from it.
type Func func(targetQPS, curQPS, prevProbability float64) (newProbability float64)

// Calculate implements Calculator interface.
func (f Func) Calculate(targetQPS, curQPS, prevProbability float64) float64 {
	return f(targetQPS, curQPS, prevProbability)
}

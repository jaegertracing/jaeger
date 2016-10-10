package metrics

// Counter tracks the number of times an event has occurred
type Counter interface {
	// Add adds the given value to the counter.
	Inc(int64)
}

// NullCounter counter that does nothing
var NullCounter Counter = nullCounter{}

type nullCounter struct{}

func (nullCounter) Inc(int64) {}

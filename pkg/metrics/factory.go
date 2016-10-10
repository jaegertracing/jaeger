package metrics

// Factory creates new metrics
type Factory interface {
	CreateCounter(name string, tags map[string]string) Counter
	CreateTimer(name string, tags map[string]string) Timer
	CreateGauge(name string, tags map[string]string) Gauge
}

// NullFactory is a metrics factory that returns NullCounter, NullTimer, and NullGauge.
var NullFactory Factory = nullFactory{}

type nullFactory struct{}

func (nullFactory) CreateCounter(name string, tags map[string]string) Counter { return NullCounter }
func (nullFactory) CreateTimer(name string, tags map[string]string) Timer     { return NullTimer }
func (nullFactory) CreateGauge(name string, tags map[string]string) Gauge     { return NullGauge }

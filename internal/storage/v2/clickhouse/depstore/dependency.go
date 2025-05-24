package depstore

import (
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

type Dependency struct {
	Parent    string    `ch:"parent"`
	Child     string    `ch:"child"`
	CallCount uint64    `ch:"call_count"`
	Source    string    `ch:"source"`
	Timestamp time.Time `ch:"timestamp"`
}

func (d *Dependency) ToModel() model.DependencyLink {
	return model.DependencyLink{
		Parent:    d.Parent,
		Child:     d.Child,
		CallCount: d.CallCount,
		Source:    d.Source,
	}.ApplyDefaults()
}

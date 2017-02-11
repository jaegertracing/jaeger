package metrics

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/pkg/cassandra"
)

// Table is a collection of metrics about Cassandra write operations.
type Table struct {
	Attempts   metrics.Counter `metric:"attempts"`
	Inserts    metrics.Counter `metric:"inserts"`
	Errors     metrics.Counter `metric:"errors"`
	LatencyOk  metrics.Timer   `metric:"latency-ok"`
	LatencyErr metrics.Timer   `metric:"latency-err"`
}

// NewTable takes a metrics scope and creates a table metrics struct
func NewTable(factory metrics.Factory, tableName string) *Table {
	t := &Table{}
	metrics.Init(t, factory.Namespace(tableName, nil), nil)
	return t
}

// Exec executes an update query and reports metrics/logs about it.
func (t *Table) Exec(query cassandra.UpdateQuery, logger zap.Logger) error {
	start := time.Now()
	err := query.Exec()
	t.Emit(err, time.Since(start))
	if err != nil {
		queryString := query.String()
		if logger != nil {
			logger.Error("Failed to exec query", zap.String("query", queryString), zap.Error(err))
		}
		return errors.Wrapf(err, "failed to Exec query '%s'", queryString)
	}
	return nil
}

// Emit will record success or failure counts and latency metrics depending on the passed error.
func (t *Table) Emit(err error, latency time.Duration) {
	t.Attempts.Inc(1)
	if err != nil {
		t.LatencyErr.Record(latency)
		t.Errors.Inc(1)
	} else {
		t.LatencyOk.Record(latency)
		t.Inserts.Inc(1)
	}
}

// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore

import (
	"database/sql"
	"encoding/binary"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

// TraceID is a serializable form of model.TraceID
type TraceID [16]byte

type SpanReader struct {
	logger *zap.Logger
	db     *sql.DB
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(logger *zap.Logger, db *sql.DB, metricsFactory metrics.Factory) spanstore.Reader {
	return storageMetrics.NewReadMetricsDecorator(newSpanReader(logger, db), metricsFactory)
}

func newSpanReader(logger *zap.Logger, db *sql.DB) *SpanReader {
	return &SpanReader{
		logger: logger,
		db:     db,
	}
}

const traceQuery = `SELECT trace_id,
						   span_id,
                           parent_id,
  						   operation_name,
  						   flags,
  						   start_time,
  						   duration,
  						   service_name
                    FROM traces WHERE trace_id = ?`

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	dbTraceID := TraceID{}
	binary.BigEndian.PutUint64(dbTraceID[:8], uint64(traceID.High))
	binary.BigEndian.PutUint64(dbTraceID[8:], uint64(traceID.Low))
	rows, err := s.db.Query(traceQuery, dbTraceID)
	if err != nil {
		return nil, err
	}

	var spans = make([]*model.Span, 0, 10)
	var traceId TraceID
	var spanId uint64
	var parentId uint64
	var operationName string
	var flags uint32
	var startTime uint64
	var duration uint64
	var serviceName string

	for rows.Next() {
		err = rows.Scan(&traceId, &spanId, &parentId, &operationName, &flags, &startTime, &duration, &serviceName)

		traceIDHigh := binary.BigEndian.Uint64(traceId[:8])
		traceIDLow := binary.BigEndian.Uint64(traceId[8:])

		span := &model.Span{
			TraceID:       model.TraceID{High: traceIDHigh, Low: traceIDLow},
			SpanID:        model.SpanID(spanId),
			ParentSpanID:  model.SpanID(parentId),
			OperationName: operationName,
			References:    nil, //TODO
			Flags:         model.Flags(flags),
			StartTime:     model.EpochMicrosecondsAsTime(startTime),
			Duration:      model.MicrosecondsAsDuration(duration),
			Tags:          nil, //TODO
			Logs:          nil, //TODO
			Process: &model.Process{
				ServiceName: serviceName,
			},
		}

		spans = append(spans, span)
	}

	return &model.Trace{
		Spans: spans,
	}, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	return nil, nil
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	return nil, nil
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, nil
}

// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/sqlite/schema"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type Configuration struct {
	dbURL string
	ttl   time.Duration
}

type Store struct {
	db     *sql.DB
	ttl    time.Duration
	logger *zap.Logger
}

var (
	insertSpanQuery    = "INSERT OR IGNORE INTO spans (trace_id, span_id, parent_id, service_name, operation_name, start_time, duration, span) VALUES (?,?,?,?,?,?,?,?)"
	insertSpanTagQuery = "INSERT OR IGNORE INTO span_tags (trace_id, span_id, tag) VALUES (?,?,?)"
	insertSpanStmt     *sql.Stmt
	insertSpanTagsStmt *sql.Stmt
)

func initDB(path string) (*sql.DB, error) {
	var needsInit bool
	if _, err := os.Stat(path); os.IsNotExist(err) {
		needsInit = true
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?%s", path, "mode=rwc&cache=shared"))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open/create specified database file: %s", path)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = MEMORY;", nil); err != nil {
		return nil, errors.Wrap(err, "cannot set journal mode")
	}

	if _, err := db.Exec("PRAGMA synchronous = OFF;", nil); err != nil {
		return nil, errors.Wrap(err, "cannot set synchronous mode")
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON;", nil); err != nil {
		return nil, errors.Wrap(err, "cannot enable foreign key constraints")
	}

	if needsInit {
		if _, err := db.Exec(schema.InitSQL, nil); err != nil {
			return nil, errors.Wrap(err, "cannot init schema")
		}
	}

	// prepare insertion statements
	insertSpanStmt, err = db.Prepare(insertSpanQuery)
	if err != nil {
		return nil, errors.Wrap(err, "error preparing span insertion statement")
	}

	insertSpanTagsStmt, err = db.Prepare(insertSpanTagQuery)
	if err != nil {
		return nil, errors.Wrap(err, "error preparing span tag insertion statement")
	}

	return db, nil
}

func WithConfiguration(config Configuration, logger *zap.Logger) *Store {
	db, err := initDB(config.dbURL)
	if err != nil {
		logger.Fatal("cannot initialize database", zap.Error(err))
	}

	s := &Store{db: db, logger: logger, ttl: config.ttl}
	s.logger.Info("sqlite storage initialized", zap.String("database", config.dbURL), zap.Float64("ttl-seconds", config.ttl.Seconds()))

	s.startCleaner()

	return s
}

func (s *Store) startCleaner() {
	cleanupQuery := fmt.Sprintf("DELETE FROM spans WHERE start_time < datetime('now', '-%d seconds')", int64(s.ttl.Seconds()))
	s.logger.Info(cleanupQuery)

	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			_, err := s.db.Exec(cleanupQuery)
			if err != nil {
				s.logger.Error("error deleting expired traces", zap.Error(err))
			}

			_, err = s.db.Exec(`PRAGMA shrink_memory;`)
			if err != nil {
				s.logger.Error("error shrinking sqlite memory", zap.Error(err))
			}
		}
	}()
}

func (s *Store) findTraceIDs(ctx context.Context, tqp *spanstore.TraceQueryParameters, tx *sql.Tx) ([]model.TraceID, error) {
	if tqp.DurationMax == 0 {
		tqp.DurationMax = time.Duration(math.MaxInt64)
	}
	s.logger.Info("tqp", zap.String("tqp", fmt.Sprintf("%+v", tqp)))

	whereClauses := []string{
		"service_name=?",
		"start_time BETWEEN ? AND ?",
		"duration BETWEEN ? AND ?",
	}

	queryValues := []interface{}{
		tqp.ServiceName,
		tqp.StartTimeMin.UTC(),
		tqp.StartTimeMax.UTC(),
		tqp.DurationMin.Nanoseconds(),
		tqp.DurationMax.Nanoseconds(),
	}

	if tqp.OperationName != "" {
		whereClauses = append(whereClauses, "operation_name=?")
		queryValues = append(queryValues, tqp.OperationName)
	}

	if len(tqp.Tags) > 0 {
		allTags := []string{}
		for k, v := range tqp.Tags {
			allTags = append(allTags, fmt.Sprintf("\"%s|||%s\"", k, v))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("(trace_id, span_id) in (SELECT trace_id, span_id FROM span_tags WHERE tag in (%s))", strings.Join(allTags, ", ")))
	}

	queryValues = append(queryValues, tqp.NumTraces)
	query := fmt.Sprintf("SELECT distinct(trace_id) FROM spans WHERE %s ORDER BY start_time DESC LIMIT ?", strings.Join(whereClauses, " AND "))

	rows, err := tx.QueryContext(ctx, query, queryValues...)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving traces")
	}
	defer rows.Close()

	traceIDs := []model.TraceID{}
	var traceIDstr string
	for rows.Next() {
		err := rows.Scan(&traceIDstr)
		if err != nil {
			return nil, errors.Wrap(err, "error reading trace ids")
		}

		traceID, err := model.TraceIDFromString(traceIDstr)
		if err != nil {
			return nil, errors.Wrap(err, "error formatting trace id from string")
		}

		traceIDs = append(traceIDs, traceID)
	}

	return traceIDs, nil
}

func (s *Store) FindTraceIDs(ctx context.Context, tqp *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()
	return s.findTraceIDs(ctx, tqp, tx)
}

func (s *Store) FindTraces(ctx context.Context, tqp *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()

	traceIDs, err := s.findTraceIDs(ctx, tqp, tx)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving trace ids")
	}

	traces := []*model.Trace{}
	for _, t := range traceIDs {
		trace, err := s.getTraceByID(ctx, t.String(), tx)
		if err != nil {
			return nil, errors.Wrap(err, "error retrieving trace")
		}
		traces = append(traces, trace)
	}

	return traces, nil
}

func (s *Store) getTraceByID(ctx context.Context, traceID string, tx *sql.Tx) (*model.Trace, error) {
	query := "SELECT span FROM spans WHERE trace_id=?"
	rows, err := tx.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving spans for trace '%s'", traceID)
	}
	defer rows.Close()

	spans := []*model.Span{}

	for rows.Next() {
		span := &model.Span{}
		var spanBytes []byte
		iErr := rows.Scan(&spanBytes)
		if iErr != nil {
			return nil, errors.Wrapf(iErr, "error reading spans for trace '%s'", traceID)
		}
		iErr = span.Unmarshal(spanBytes)
		if iErr != nil {
			return nil, errors.Wrapf(iErr, "error unmarshaling span from proto '%s'", traceID)
		}

		spans = append(spans, span)
	}

	if len(spans) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}

	for _, span := range spans {
		span.Tags, err = s.getSpanTags(ctx, traceID, span.SpanID.String(), tx)
		if err != nil {
			return nil, errors.Wrap(err, "error retrieving span tags")
		}
	}

	return &model.Trace{
		Spans: spans,
		ProcessMap: []model.Trace_ProcessMapping{
			model.Trace_ProcessMapping{
				ProcessID: spans[0].ProcessID,
				Process:   *spans[0].Process,
			},
		},
	}, nil
}

func (s *Store) getSpanTags(ctx context.Context, traceID, spanID string, tx *sql.Tx) ([]model.KeyValue, error) {
	rows, err := tx.QueryContext(ctx, "SELECT tag FROM span_tags WHERE trace_id=? AND span_id=?", traceID, spanID)
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving tags for span '%s'", spanID)
	}
	defer rows.Close()

	var tag string
	spanTags := []model.KeyValue{}

	for rows.Next() {
		err := rows.Scan(&tag)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing tag for span '%s'", spanID)
		}

		kv := strings.Split(tag, "|||")
		spanTags = append(spanTags, model.KeyValue{
			Key:  kv[0],
			VStr: kv[1],
		})
	}

	return spanTags, nil
}

func (s *Store) GetOperations(ctx context.Context, service string) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, "SELECT distinct(operation_name) FROM spans WHERE service_name=?", service)
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving operations for service %s", service)
	}
	defer rows.Close()

	operationNames := []string{}
	var operationName string
	for rows.Next() {
		err := rows.Scan(&operationName)
		if err != nil {
			return nil, errors.Wrap(err, "error reading operation name")
		}
		operationNames = append(operationNames, operationName)
	}

	return operationNames, nil
}

func (s *Store) GetServices(ctx context.Context) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, "SELECT distinct(service_name) FROM spans")
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving service names")
	}
	defer rows.Close()

	serviceNames := []string{}
	var serviceName string
	for rows.Next() {
		err := rows.Scan(&serviceName)
		if err != nil {
			return nil, errors.Wrap(err, "error reading service name")
		}
		serviceNames = append(serviceNames, serviceName)
	}

	return serviceNames, nil
}

func (s *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()

	return s.getTraceByID(ctx, traceID.String(), tx)
}

func (s *Store) WriteSpan(span *model.Span) error {
	spanProto, err := proto.Marshal(span)
	if err != nil {
		return errors.Wrap(err, "error marshaling span to protobuf")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return errors.Wrap(err, "error starting transaction")
	}
	defer tx.Rollback()

	stmt := tx.Stmt(insertSpanStmt)
	_, err = stmt.Exec(span.TraceID.String(), span.SpanID.String(), span.ParentSpanID().String(),
		span.Process.ServiceName, span.OperationName, span.StartTime, span.Duration.Nanoseconds(), spanProto)
	if err != nil {
		return errors.Wrap(err, "error saving span")
	}

	stmt = tx.Stmt(insertSpanTagsStmt)
	for _, kv := range span.Tags {
		var tag string
		switch kv.VType {
		case model.ValueType_INT64:
			tag = fmt.Sprintf("%s|||%d", kv.Key, kv.VInt64)
		case model.ValueType_BOOL:
			tag = fmt.Sprintf("%s|||%t", kv.Key, kv.VBool)
		case model.ValueType_STRING:
			tag = fmt.Sprintf("%s|||%s", kv.Key, kv.VStr)
		case model.ValueType_FLOAT64:
			tag = fmt.Sprintf("%s|||%f", kv.Key, kv.VFloat64)
		}

		_, err = stmt.Exec(span.TraceID.String(), span.SpanID.String(), tag)
		if err != nil {
			return errors.Wrap(err, "error saving span tags")
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "error commiting transaction")
	}

	return nil
}

func (s *Store) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return []model.DependencyLink{}, nil
}

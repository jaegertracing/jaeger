// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type TestRows interface {
	driver.Rows
	Reset()
}

type QueryResponse struct {
	Rows TestRows
	Err  error
}

func (r *QueryResponse) Reset() {
	if r.Rows != nil {
		r.Rows.Reset()
	}
}

type BatchResponse struct {
	Batch *Batch
	Err   error
}

type Driver struct {
	driver.Conn

	T               *testing.T
	QueryResponses  map[string]*QueryResponse
	BatchResponses  map[string]*BatchResponse
	RecordedQueries []string
}

func (d *Driver) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	d.RecordedQueries = append(d.RecordedQueries, query)

	// Normalize whitespace so substring matching works regardless of indentation.
	normalized := strings.Join(strings.Fields(query), " ")
	for querySubstring, response := range d.QueryResponses {
		normalizedSubstring := strings.Join(strings.Fields(querySubstring), " ")
		if strings.Contains(normalized, normalizedSubstring) {
			return response.Rows, response.Err
		}
	}

	return nil, nil
}

func (d *Driver) PrepareBatch(
	_ context.Context,
	query string,
	_ ...driver.PrepareBatchOption,
) (driver.Batch, error) {
	d.RecordedQueries = append(d.RecordedQueries, query)

	for querySubstring, response := range d.BatchResponses {
		if strings.Contains(query, querySubstring) {
			return response.Batch, response.Err
		}
	}

	return nil, nil
}

type Batch struct {
	driver.Batch
	T          *testing.T
	Appended   [][]any
	AppendErr  error
	SendCalled bool
	SendErr    error
}

func (b *Batch) Append(v ...any) error {
	if b.AppendErr != nil {
		return b.AppendErr
	}
	b.Appended = append(b.Appended, v)
	return nil
}

func (b *Batch) Send() error {
	if b.SendErr != nil {
		return b.SendErr
	}
	b.SendCalled = true
	return nil
}

func (*Batch) Close() error {
	return nil
}

type Rows[T any] struct {
	driver.Rows

	Data     []T
	Index    int
	ScanErr  error
	ScanFn   func(dest any, src T) error
	CloseErr error
	RowsErr  error
}

func (r *Rows[T]) Reset() {
	r.Index = 0
}

func (r *Rows[T]) Close() error {
	return r.CloseErr
}

func (r *Rows[T]) Err() error {
	return r.RowsErr
}

func (r *Rows[T]) Next() bool {
	return r.Index < len(r.Data)
}

func (r *Rows[T]) ScanStruct(dest any) error {
	if r.ScanErr != nil {
		return r.ScanErr
	}
	if r.Index >= len(r.Data) {
		return errors.New("no more rows")
	}
	if r.ScanFn == nil {
		return errors.New("ScanFn is not provided")
	}
	err := r.ScanFn(dest, r.Data[r.Index])
	r.Index++
	return err
}

func (r *Rows[T]) Scan(dest ...any) error {
	if r.ScanErr != nil {
		return r.ScanErr
	}
	if r.Index >= len(r.Data) {
		return errors.New("no more rows")
	}
	if r.ScanFn == nil {
		return errors.New("ScanFn is not provided")
	}
	err := r.ScanFn(dest, r.Data[r.Index])
	r.Index++
	return err
}

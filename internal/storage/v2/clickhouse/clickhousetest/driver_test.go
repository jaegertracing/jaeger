// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriver_Query_Match(t *testing.T) {
	rows := &Rows[string]{Data: []string{"a"}}
	d := &Driver{
		QueryResponses: map[string]*QueryResponse{
			"SELECT foo": {Rows: rows, Err: nil},
		},
	}
	got, err := d.Query(context.Background(), "SELECT foo FROM bar")
	require.NoError(t, err)
	assert.Equal(t, rows, got)
	assert.Equal(t, []string{"SELECT foo FROM bar"}, d.RecordedQueries)
}

func TestDriver_Query_MatchError(t *testing.T) {
	wantErr := errors.New("query error")
	d := &Driver{
		QueryResponses: map[string]*QueryResponse{
			"SELECT foo": {Rows: nil, Err: wantErr},
		},
	}
	got, err := d.Query(context.Background(), "SELECT foo FROM bar")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, got)
}

func TestDriver_Query_NoMatch(t *testing.T) {
	d := &Driver{
		QueryResponses: map[string]*QueryResponse{
			"SELECT foo": {Rows: &Rows[string]{}, Err: nil},
		},
	}
	got, err := d.Query(context.Background(), "SELECT baz FROM bar")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDriver_Query_WhitespaceNormalization(t *testing.T) {
	rows := &Rows[string]{Data: []string{"a"}}
	d := &Driver{
		QueryResponses: map[string]*QueryResponse{
			"SELECT   foo": {Rows: rows},
		},
	}
	got, err := d.Query(context.Background(), "SELECT foo FROM bar")
	require.NoError(t, err)
	assert.Equal(t, rows, got)
}

func TestDriver_PrepareBatch_Match(t *testing.T) {
	batch := &Batch{}
	d := &Driver{
		BatchResponses: map[string]*BatchResponse{
			"INSERT INTO spans": {Batch: batch, Err: nil},
		},
	}
	got, err := d.PrepareBatch(context.Background(), "INSERT INTO spans VALUES (?)")
	require.NoError(t, err)
	assert.Equal(t, batch, got)
	assert.Equal(t, []string{"INSERT INTO spans VALUES (?)"}, d.RecordedQueries)
}

func TestDriver_PrepareBatch_MatchError(t *testing.T) {
	wantErr := errors.New("batch error")
	d := &Driver{
		BatchResponses: map[string]*BatchResponse{
			"INSERT INTO spans": {Batch: nil, Err: wantErr},
		},
	}
	got, err := d.PrepareBatch(context.Background(), "INSERT INTO spans VALUES (?)")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, got)
}

func TestDriver_PrepareBatch_NoMatch(t *testing.T) {
	d := &Driver{
		BatchResponses: map[string]*BatchResponse{
			"INSERT INTO spans": {Batch: &Batch{}},
		},
	}
	got, err := d.PrepareBatch(context.Background(), "INSERT INTO other VALUES (?)")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBatch_Append(t *testing.T) {
	b := &Batch{}
	require.NoError(t, b.Append("row1", 42))
	assert.Equal(t, [][]any{{"row1", 42}}, b.Appended)
}

func TestBatch_Append_Error(t *testing.T) {
	wantErr := errors.New("append error")
	b := &Batch{AppendErr: wantErr}
	require.ErrorIs(t, b.Append("row1"), wantErr)
	assert.Empty(t, b.Appended)
}

func TestBatch_Send(t *testing.T) {
	b := &Batch{}
	require.NoError(t, b.Send())
	assert.True(t, b.SendCalled)
}

func TestBatch_Send_Error(t *testing.T) {
	wantErr := errors.New("send error")
	b := &Batch{SendErr: wantErr}
	require.ErrorIs(t, b.Send(), wantErr)
	assert.False(t, b.SendCalled)
}

func TestBatch_Close(t *testing.T) {
	b := &Batch{}
	require.NoError(t, b.Close())
}

func TestRows_Next(t *testing.T) {
	r := &Rows[string]{Data: []string{"a", "b"}}
	assert.True(t, r.Next())
	assert.True(t, r.Next())
	assert.False(t, r.Next())
}

func TestRows_Close(t *testing.T) {
	r := &Rows[string]{}
	require.NoError(t, r.Close())

	r2 := &Rows[string]{CloseErr: errors.New("close error")}
	require.Error(t, r2.Close())
}

func TestRows_Err(t *testing.T) {
	r := &Rows[string]{}
	require.NoError(t, r.Err())

	wantErr := errors.New("rows error")
	r2 := &Rows[string]{RowsErr: wantErr}
	require.ErrorIs(t, r2.Err(), wantErr)
}

func TestRows_ScanStruct(t *testing.T) {
	r := &Rows[string]{
		Data:   []string{"hello"},
		ScanFn: func(dest any, src string) error { *(dest.(*string)) = src; return nil },
	}
	r.Next()
	var out string
	require.NoError(t, r.ScanStruct(&out))
	assert.Equal(t, "hello", out)
}

func TestRows_ScanStruct_ScanErr(t *testing.T) {
	wantErr := errors.New("scan error")
	r := &Rows[string]{Data: []string{"x"}, ScanErr: wantErr}
	r.Next()
	require.ErrorIs(t, r.ScanStruct(nil), wantErr)
}

func TestRows_ScanStruct_OutOfBounds(t *testing.T) {
	r := &Rows[string]{Data: []string{"x"}}
	// Index is 0 (Next not called), so out-of-bounds
	require.Error(t, r.ScanStruct(nil))
}

func TestRows_ScanStruct_NilScanFn(t *testing.T) {
	r := &Rows[string]{Data: []string{"x"}}
	r.Next()
	require.Error(t, r.ScanStruct(nil))
}

func TestRows_Scan(t *testing.T) {
	r := &Rows[string]{
		Data:   []string{"hello"},
		ScanFn: func(dest any, src string) error { dest.([]any)[0] = src; return nil },
	}
	r.Next()
	var out any
	require.NoError(t, r.Scan(&out))
}

func TestRows_Scan_ScanErr(t *testing.T) {
	wantErr := errors.New("scan error")
	r := &Rows[string]{Data: []string{"x"}, ScanErr: wantErr}
	r.Next()
	require.ErrorIs(t, r.Scan(nil), wantErr)
}

func TestRows_Scan_OutOfBounds(t *testing.T) {
	r := &Rows[string]{Data: []string{"x"}}
	require.Error(t, r.Scan(nil))
}

func TestRows_Scan_NilScanFn(t *testing.T) {
	r := &Rows[string]{Data: []string{"x"}}
	r.Next()
	require.Error(t, r.Scan(nil))
}

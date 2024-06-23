// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	//"github.com/jaegertracing/jaeger/pkg/testutils"
)

// Comment GoLeak check for now to focus on other tests first.
// Will fix in a later commit.
/*func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}*/

func TestExportSpans(t *testing.T) {
	t.Run("create factory and export spans", func(t *testing.T) {
		var items int
		initClickhouseTestServer(t, func(query string, values []driver.Value) error {
			if strings.HasPrefix(query, "INSERT") {
				items++
				require.Equal(t, "test-operation", values[4])
				require.Equal(t, "test-service", values[5])
				require.Equal(t, []string{"attKey0", "attKey1"}, values[6])
				require.Equal(t, []string{"attVal0", "attVal1"}, values[7])
			}
			return nil
		})

		c := Config{
			Endpoint:       "clickhouse://127.0.0.1:9000",
			SpansTableName: "jaeger_spans",
		}

		f := NewFactory(context.TODO(), c, zap.NewNop())
		require.NotNil(t, f.client)

		err := f.ChExportSpans(context.TODO(), createTraces(5))
		require.NoError(t, err)

		err = f.ChExportSpans(context.TODO(), createTraces(10))
		require.NoError(t, err)

		require.Equal(t, 15, items)
	})
}

func createTraces(count int) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()
	for i := 0; i < count; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		s.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		s.Attributes().PutStr("attKey0", "attVal0")
		s.Attributes().PutStr("attKey1", "attVal1")
		s.SetName("test-operation")
	}
	return traces
}

func initClickhouseTestServer(_ *testing.T, recorder recorder) {
	driverName = "test"
	sql.Register(driverName, &testClickhouseDriver{
		recorder: recorder,
	})
}

type recorder func(query string, values []driver.Value) error

type testClickhouseDriver struct {
	recorder recorder
}

func (t *testClickhouseDriver) Open(_ string) (driver.Conn, error) {
	return &testClickhouseDriverConn{
		recorder: t.recorder,
	}, nil
}

type testClickhouseDriverConn struct {
	recorder recorder
}

func (*testClickhouseDriverConn) Begin() (driver.Tx, error) {
	return &testClickhouseDriverTx{}, nil
}

func (*testClickhouseDriverConn) Close() error {
	return nil
}

func (t *testClickhouseDriverConn) Prepare(query string) (driver.Stmt, error) {
	return &testClickhouseDriverStmt{
		query:    query,
		recorder: t.recorder,
	}, nil
}

func (*testClickhouseDriverConn) CheckNamedValue(_ *driver.NamedValue) error {
	return nil
}

type testClickhouseDriverTx struct{}

func (*testClickhouseDriverTx) Commit() error {
	return nil
}

func (*testClickhouseDriverTx) Rollback() error {
	return nil
}

type testClickhouseDriverStmt struct {
	query    string
	recorder recorder
}

func (*testClickhouseDriverStmt) Close() error {
	return nil
}

func (t *testClickhouseDriverStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, t.recorder(t.query, args)
}

func (t *testClickhouseDriverStmt) NumInput() int {
	return strings.Count(t.query, "?")
}

func (*testClickhouseDriverStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return nil, nil
}

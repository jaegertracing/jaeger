// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

import (
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/ClickHouse/clickhouse-go/v2"
	chproto "github.com/ClickHouse/clickhouse-go/v2/lib/proto"
)

var (
	PingQuery      = "SELECT 1"
	HandshakeQuery = "SELECT displayName(), version(), revision(), timezone()"
)

// FailureConfig is a map of query body to error
type FailureConfig map[string]error

// NewServer creates a new HTTP test server that simulates a ClickHouse server.
// It should only be used in tests.
func NewServer(failures FailureConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		query := string(body)

		block := chproto.NewBlock()

		if err, shouldFail := failures[query]; shouldFail {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		switch query {
		case PingQuery:
			block.AddColumn("1", "UInt8")
			block.Append(uint8(1))
		case HandshakeQuery:
			block.AddColumn("displayName()", "String")
			block.AddColumn("version()", "String")
			block.AddColumn("revision()", "UInt32")
			block.AddColumn("timezone()", "String")
			block.Append("mock-server", "23.3.1", chproto.DBMS_MIN_REVISION_WITH_CUSTOM_SERIALIZATION, "UTC")
		default:
		}

		var buf proto.Buffer
		block.Encode(&buf, clickhouse.ClientTCPProtocolVersion)

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(buf.Buf)
	}))
}

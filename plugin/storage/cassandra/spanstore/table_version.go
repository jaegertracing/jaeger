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

package spanstore

import (
	"context"
	"fmt"

	otlog "github.com/opentracing/opentracing-go/log"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

const queryTraceTableVersion = `SELECT * FROM traces_v2 LIMIT 1;`

// Returns the version of trace table used in the jaeger cassandra backend. Remove this after we permanently switch to traces_v2.
func getTraceTableVersion(ctx context.Context, s cassandra.Session) dbmodel.TraceVersion {
	span, _ := startSpanForQuery(ctx, "traceTableVersion", queryTraceTableVersion)
	defer span.Finish()
	if err := s.Query(queryTraceTableVersion).Exec(); err != nil {
		span.LogFields(otlog.String("event", fmt.Sprintf("trace table version : %v", dbmodel.V1)))
		return dbmodel.V1
	}
	span.LogFields(otlog.String("event", fmt.Sprintf("trace table version : %v", dbmodel.V2)))
	return dbmodel.V2
}

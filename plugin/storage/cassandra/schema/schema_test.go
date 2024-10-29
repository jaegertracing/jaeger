// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryCreationFromTemplate(t *testing.T) {
	cfg := DefaultSchema()
	res, err := getQueryFileAsBytes(`v004-go-tmpl-test.cql.tmpl`, &cfg)
	require.NoError(t, err)

	queryStrings, err := getQueriesFromBytes(res)
	require.NoError(t, err)

	expOutputQueries := []string{
		`CREATE TYPE IF NOT EXISTS jaeger_v2_test.keyvalue (
key             text,
value_type      text,
value_string    text,
value_bool      boolean,
value_long      bigint,
value_double    double,
value_binary    blob
);
`,
		`CREATE TYPE IF NOT EXISTS jaeger_v2_test.log (
ts      bigint,
fields  frozen<list<frozen<jaeger_v2_test.keyvalue>>>
);
`,
		`CREATE TABLE IF NOT EXISTS jaeger_v2_test.service_names (
service_name text,
PRIMARY KEY (service_name)
)
WITH compaction = {
'min_threshold': '4',
'max_threshold': '32',
'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy'
}
AND default_time_to_live = 172800
AND speculative_retry = 'NONE'
AND gc_grace_seconds = 10800;
`,
		`CREATE TABLE IF NOT EXISTS jaeger_v2_test.dependencies_v2 (
ts_bucket    timestamp,
ts           timestamp,
dependencies list<frozen<dependency>>,
PRIMARY KEY (ts_bucket, ts)
) WITH CLUSTERING ORDER BY (ts DESC)
AND compaction = {
'min_threshold': '4',
'max_threshold': '32',
'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy'
}
AND default_time_to_live = 0;
`,
	}

	assert.Equal(t, len(expOutputQueries), len(queryStrings))

	for i := range expOutputQueries {
		assert.Equal(t, expOutputQueries[i], queryStrings[i])
	}
}

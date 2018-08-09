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

import "fmt"

// TODO: resolve traceID concerns (may not require any changes here)
const mapping = `{
	"settings":{
		"index.number_of_shards": ${__NUMBER_OF_SHARDS__},
		"index.number_of_replicas": ${__NUMBER_OF_REPLICAS__},
		"index.mapping.nested_fields.limit":50,
		"index.requests.cache.enable":true,
		"index.mapper.dynamic":false
	},
	"mappings":{
		"_default_":{
			"_all":{
				"enabled":false
			}
		},
		"%s":%s
	}
}`

var (
	spanMapping = fmt.Sprintf(
		mapping,
		spanType,
		`{
	"properties": {
		"traceID": {
			"type": "keyword",
			"ignore_above": 256
		},
		"parentSpanID": {
			"type": "keyword",
			"ignore_above": 256
		},
		"spanID": {
			"type": "keyword",
			"ignore_above": 256
		},
		"operationName": {
			"type": "keyword",
			"ignore_above": 256
		},
		"startTime": {
			"type": "long"
		},
		"startTimeMillis": {
			"type": "date",
			"format": "epoch_millis"
		},
		"duration": {
			"type": "long"
		},
		"flags": {
			"type": "integer"
		},
		"logs": {
			"properties": {
				"timestamp": {
					"type": "long"
				},
				"fields": {
					"type": "nested",
					"dynamic": false,
					"properties": {
						"key": {
							"type": "keyword",
							"ignore_above": 256
						},
						"value": {
							"type": "keyword",
							"ignore_above": 256
						},
						"tagType": {
							"type": "keyword",
							"ignore_above": 256
						}
					}
				}
			}
		},
		"process": {
			"properties": {
				"serviceName": {
					"type": "keyword",
					"ignore_above": 256
				},
				"tags": {
					"type": "nested",
					"dynamic": false,
					"properties": {
						"key": {
							"type": "keyword",
							"ignore_above": 256
						},
						"value": {
							"type": "keyword",
							"ignore_above": 256
						},
						"tagType": {
							"type": "keyword",
							"ignore_above": 256
						}
					}
				}
			}
		},
		"references": {
			"type": "nested",
			"dynamic": false,
			"properties": {
				"refType": {
					"type": "keyword",
					"ignore_above": 256
				},
				"traceID": {
					"type": "keyword",
					"ignore_above": 256
				},
				"spanID": {
					"type": "keyword",
					"ignore_above": 256
				}
			}
		},
		"tags": {
			"type": "nested",
			"dynamic": false,
			"properties": {
				"key": {
					"type": "keyword",
					"ignore_above": 256
				},
				"value": {
					"type": "keyword",
					"ignore_above": 256
				},
				"tagType": {
					"type": "keyword",
					"ignore_above": 256
				}
			}
		}
	}
}`)

	serviceMapping = fmt.Sprintf(
		mapping,
		serviceType,
		`{
	"properties":{
		"serviceName":{
			"type":"keyword",
			"ignore_above":256
		},
		"operationName":{
			"type":"keyword",
			"ignore_above":256
		}
	}
}`)
)

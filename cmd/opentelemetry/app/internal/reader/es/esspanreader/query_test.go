// Copyright (c) 2020 The Jaeger Authors.
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

package esspanreader

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	defaultMaxDocCount        = 10_000
	servicesSearchBodyFixture = `{
  "aggs": {
    "serviceName": {
      "terms": {
        "field": "serviceName",
        "size": 10000
      }
    }
  },
  "size": 0,
  "terminate_after": 0
}`
	operationsSearchBodyFixture = `{
  "aggs": {
    "operationName": {
      "terms": {
        "field": "operationName",
        "size": 10000
      }
    }
  },
  "query": {
    "term": {
      "serviceName": {
        "value": "foo"
      }
    }
  },
  "size": 0,
  "terminate_after": 0
}`
	findTraceIDsSearchBodyFixture = `{
  "aggs": {
    "traceID": {
      "aggs": {
        "startTime": {
          "max": {
            "field": "startTime"
          }
        }
      },
      "terms": {
        "field": "traceID",
        "size": 0,
        "order": {
          "startTime": "desc"
        }
      }
    }
  },
  "query": {
    "bool": {
      "must": [
        {
          "range": {
            "startTime": {
              "gte": 18439948709130680271,
              "lte": 18439948709130680271
            }
          }
        },
        {
          "range": {
            "duration": {
              "gte": 1000000,
              "lte": 60000000
            }
          }
        },
        {
          "term": {
            "process.serviceName": "foo"
          }
        },
        {
          "term": {
            "operationName": "bar"
          }
        },
        {
          "bool": {
            "should": [
              {
                "bool": {
                  "must": [
                    {
                      "regexp": {
                        "tag.key": {
                          "value": "val"
                        }
                      }
                    }
                  ]
                }
              },
              {
                "bool": {
                  "must": [
                    {
                      "regexp": {
                        "process.tag.key": {
                          "value": "val"
                        }
                      }
                    }
                  ]
                }
              },
              {
                "nested": {
                  "path": "tags",
                  "query": {
                    "bool": {
                      "must": [
                        {
                          "match": {
                            "tags.key": {
                              "query": "key"
                            }
                          }
                        },
                        {
                          "regexp": {
                            "tags.value": {
                              "value": "val"
                            }
                          }
                        }
                      ]
                    }
                  }
                }
              },
              {
                "nested": {
                  "path": "process.tags",
                  "query": {
                    "bool": {
                      "must": [
                        {
                          "match": {
                            "process.tags.key": {
                              "query": "key"
                            }
                          }
                        },
                        {
                          "regexp": {
                            "process.tags.value": {
                              "value": "val"
                            }
                          }
                        }
                      ]
                    }
                  }
                }
              },
              {
                "nested": {
                  "path": "logs.fields",
                  "query": {
                    "bool": {
                      "must": [
                        {
                          "match": {
                            "logs.fields.key": {
                              "query": "key"
                            }
                          }
                        },
                        {
                          "regexp": {
                            "logs.fields.value": {
                              "value": "val"
                            }
                          }
                        }
                      ]
                    }
                  }
                }
              }
            ]
          }
        }
      ]
    }
  },
  "size": 0,
  "terminate_after": 0
}`

	findTraceIDQuery = `{
  "term": {
    "traceID": {
      "value": "000000000000aaaa"
    }
  }
}`
)

func TestGetServicesSearchBody(t *testing.T) {
	sb := getServicesSearchBody(defaultMaxDocCount)
	jsonQuery, err := json.MarshalIndent(sb, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, servicesSearchBodyFixture, string(jsonQuery))
}

func TestGetOperationsSearchBody(t *testing.T) {
	sb := getOperationsSearchBody("foo", defaultMaxDocCount)
	jsonQuery, err := json.MarshalIndent(sb, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, operationsSearchBodyFixture, string(jsonQuery))
}

func TestFindTraceIDsSearchBody(t *testing.T) {
	q := &spanstore.TraceQueryParameters{
		ServiceName:   "foo",
		OperationName: "bar",
		DurationMin:   time.Second,
		DurationMax:   time.Minute,
		Tags:          map[string]string{"key": "val"},
	}
	sb := findTraceIDsSearchBody(dbmodel.NewToDomain("@"), q)
	jsonQuery, err := json.MarshalIndent(sb, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, findTraceIDsSearchBodyFixture, string(jsonQuery))
}

func TestTraceIDQuery(t *testing.T) {
	traceID, err := model.TraceIDFromString("AAAA")
	require.NoError(t, err)
	query := traceIDQuery(traceID)
	jsonQuery, err := json.MarshalIndent(query, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, findTraceIDQuery, string(jsonQuery))
}

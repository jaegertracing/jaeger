// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

const spanMapping = `{
   "settings":{
      "index.mapping.nested_fields.limit":50,
      "index.requests.cache.enable":true,
      "index.mapper.dynamic":false,
      "analysis":{
         "analyzer":{
            "traceId_analyzer":{
               "type":"custom",
               "tokenizer":"keyword",
               "filter":"traceId_filter"
            }
         },
         "filter":{
            "traceId_filter":{
               "type":"pattern_capture",
               "patterns":[
                  "([0-9a-f]{1,16})$"
               ],
               "preserve_original":true
            }
         }
      }
   },
   "mappings":{
      "_default_":{
         "_all":{
            "enabled":false
         }
      },
      "span":{
         "properties":{
            "traceID":{
               "type":"string",
               "analyzer":"traceId_analyzer",
               "fielddata":"true"
            },
            "parentSpanID":{
               "type":"keyword",
               "ignore_above":256
            },
            "spanID":{
               "type":"keyword",
               "ignore_above":256
            },
            "operationName":{
               "type":"keyword",
               "ignore_above":256
            },
            "startTime":{
               "type":"long"
            },
            "duration":{
               "type":"long"
            },
            "flags":{
               "type":"integer"
            },
            "logs":{
               "properties":{
                  "timestamp":{
                     "type":"long"
                  },
                  "tags":{
                     "type":"nested",
                     "dynamic":false,
                     "properties":{
                        "key":{
                           "type":"keyword",
                           "ignore_above":256
                        },
                        "value":{
                           "type":"keyword",
                           "ignore_above":256
                        },
                        "tagType":{
                           "type":"keyword",
                           "ignore_above":256
                        }
                     }
                  }
               }
            },
            "process":{
               "properties":{
                  "serviceName":{
                     "type":"keyword",
                     "ignore_above":256
                  },
                  "tags":{
                     "type":"nested",
                     "dynamic":false,
                     "properties":{
                        "key":{
                           "type":"keyword",
                           "ignore_above":256
                        },
                        "value":{
                           "type":"keyword",
                           "ignore_above":256
                        },
                        "tagType":{
                           "type":"keyword",
                           "ignore_above":256
                        }
                     }
                  }
               }
            },
            "references":{
               "type":"nested",
               "dynamic":false,
               "properties":{
                  "refType":{
                     "type":"keyword",
                     "ignore_above":256
                  },
                  "traceID":{
                     "type":"keyword",
                     "ignore_above":256
                  },
                  "spanID":{
                     "type":"keyword",
                     "ignore_above":256
                  }
               }
            },
            "tags":{
               "type":"nested",
               "dynamic":false,
               "properties":{
                  "key":{
                     "type":"keyword",
                     "ignore_above":256
                  },
                  "value":{
                     "type":"keyword",
                     "ignore_above":256
                  },
                  "tagType":{
                     "type":"keyword",
                     "ignore_above":256
                  }
               }
            }
         }
      },
      "service":{
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
      }
   }
}`

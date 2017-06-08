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

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"github.com/pkg/errors"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/model/converter/json"
	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/es"
)

const spanType = "span"
const serviceType = "service"
const hostPort = "http://localhost:9200"
const spanMapping = `{
  "settings": {
    "index.mapping.nested_fields.limit": 	50,
    "index.requests.cache.enable": 		true,
    "index.mapper.dynamic":           false,
    "analysis": {
      "analyzer": {
        "traceId_analyzer": {
          "type": 	"custom",
          "tokenizer":	"keyword",
          "filter":	"traceId_filter"
        }
      },
      "filter": {
        "traceId_filter": {
          "type":		"pattern_capture",
          "patterns": 		["([0-9a-f]{1,16})$"],
          "preserve_original": 	true
        }
      }
    }
  },

  "mappings": {
    "_default_": {
      "_all": 	{ "enabled" : false }
    },
    "span": {
      "properties": {
        "traceID": 		{ "type": "string", "analyzer": "traceId_analyzer", "fielddata": "true" },
        "parentSpanID": 	{ "type": "keyword", "ignore_above": 256 },
        "spanID": 		{ "type": "keyword", "ignore_above": 256 },
        "operationName": 	{ "type": "keyword", "ignore_above": 256 },
        "startTime": 		{ "type": "long" },
        "duration": 		{ "type": "long" },
        "flags": 		{ "type": "integer" },
        "logs": {
          "properties": {
            "timestamp": 	{ "type": "long" },
            "tags": {
              "type": 		"nested",
              "dynamic":  	false,
              "properties": {
                "key": 		{ "type": "keyword", "ignore_above": 256 },
                "value": 	{ "type": "keyword", "ignore_above": 256 },
                "tagType": 	{ "type": "keyword", "ignore_above": 256 }
              }
            }
          }
        },
        "process": {
          "properties": {
            "serviceName": 	{ "type": "keyword", "ignore_above": 256 },
            "tags": {
              "type": 		"nested",
              "dynamic":  	false,
              "properties": {
                "key": 		{ "type": "keyword", "ignore_above": 256 },
                "value": 	{ "type": "keyword", "ignore_above": 256 },
                "tagType": 	{ "type": "keyword", "ignore_above": 256 }
              }
            }
          }
        },
        "references": {
          "type":   		"nested",
          "dynamic": 		false,
          "properties": {
            "refType": 		{ "type": "keyword", "ignore_above": 256 },
            "traceID": 		{ "type": "keyword", "ignore_above": 256 },
            "spanID": 		{ "type": "keyword", "ignore_above": 256 },
          }
        },
        "tags": {
          "type": 		"nested",
          "dynamic": false,
          "properties": {
            "key": 		{ "type": "keyword", "ignore_above": 256 },
            "value": 		{ "type": "keyword", "ignore_above": 256 },
            "tagType": 		{ "type": "keyword", "ignore_above": 256 }
          }
        }
      }
    },
    "service": {
      "properties": {
        "serviceName": 		{ "type": "keyword", "ignore_above": 256 },
        "operationName": 	{ "type": "keyword", "ignore_above": 256 }
      }
    }
  }
}`

type SpanWriter struct {
	client es.Client
	logger *zap.Logger
}

type Service struct {
	serviceName   string
	operationName string
}

func NewSpanWriter(
	client es.Client,
	logger *zap.Logger,
) *SpanWriter {
	return &SpanWriter{
		client: client,
		logger: logger,
	}
}

func (s *SpanWriter) WriteSpan(span *model.Span) error {
	// Convert model.Span into json.Span
	jsonSpan := json.FromDomainEmbedProcess(span)

	ctx := context.Background()

	today := time.Now().Format("1995-04-21")
	jaegerIndexName := "jaeger-" + today

	// Check if index exists, and create index if it does not.
	// TODO: We don't need to check every write. Try to pull this out of WriteSpan.
	exists, err := s.client.IndexExists(jaegerIndexName).Do(ctx)
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to find index", s.logger)
	}
	if !exists {
		_, err = s.client.CreateIndex(jaegerIndexName).Body(spanMapping).Do(ctx)
		if err != nil {
			return s.logError(jsonSpan, err, "Failed to create index", s.logger)
		}
	}

	// Insert serviceName:operationName document
	service := Service{
		serviceName:   span.Process.ServiceName,
		operationName: span.OperationName,
	}
	serviceID := fmt.Sprintf("%s|%s", service.serviceName, service.operationName)
	_, err = s.client.Index().Index(jaegerIndexName).Type(serviceType).Id(serviceID).BodyJson(service).Do(ctx)
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to insert service:operation", s.logger)
	}

	// Insert json.Span document
	_, err = s.client.Index().Index(jaegerIndexName).Type(spanType).BodyJson(jsonSpan).Do(ctx)
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to insert span", s.logger)
	}
	return nil
}

func (s *SpanWriter) logError(span *jModel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
	With(zap.String("trace_id", string(span.TraceID))).
		With(zap.String("span_id", string(span.SpanID))).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}
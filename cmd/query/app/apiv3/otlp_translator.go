// Copyright (c) 2021 The Jaeger Authors.
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

package apiv3

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/model/pdata"
	semconv "go.opentelemetry.io/collector/model/semconv/v1.5.0"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	commonv1 "github.com/jaegertracing/jaeger/proto-gen/otel/common/v1"
	resourcev1 "github.com/jaegertracing/jaeger/proto-gen/otel/resource/v1"
	tracev1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
)

const (
	tagStatusCode = "status.code"
	tagStatusMsg  = "status.message"

	tagSpanKind      = "span.kind"
	tagError         = "error"
	tagMessage       = "message"
	tagHTTPStatusMsg = "http.status_message"
	tagW3CTraceState = "w3c.tracestate"
)

// OpenTelemetry collector implements translator from Jaeger model to pdata (wrapper around OTLP).
// However, it cannot be used because the imported OTLP in the translator is in the collector's private package.
func jaegerSpansToOTLP(spans []*model.Span) []*tracev1.ResourceSpans {
	spansByLibrary := make(map[resource]map[instrumentationLibrary]*tracev1.InstrumentationLibrarySpans)
	for _, s := range spans {
		otlpSpan, res, library := jSpanToOTLP(s)
		resourceSpans, ok := spansByLibrary[res]
		if !ok {
			resourceSpans = map[instrumentationLibrary]*tracev1.InstrumentationLibrarySpans{}
			resourceSpans[library] = &tracev1.InstrumentationLibrarySpans{
				InstrumentationLibrary: &commonv1.InstrumentationLibrary{
					Name:    library.name,
					Version: library.version,
				},
			}
			spansByLibrary[res] = resourceSpans
		}
		resourceSpans[library].Spans = append(resourceSpans[library].GetSpans(), otlpSpan)
	}

	var rss []*tracev1.ResourceSpans
	for res, libMap := range spansByLibrary {
		rs := &tracev1.ResourceSpans{
			Resource: res.resource,
		}
		for _, v := range libMap {
			rs.InstrumentationLibrarySpans = append(rs.InstrumentationLibrarySpans, v)
		}
		rss = append(rss, rs)
	}
	return rss
}

type instrumentationLibrary struct {
	name, version string
}

// helper type used as a map key
type resource struct {
	serviceName string
	// concatenated and hashed string tags
	// to make sure services are uniquely grouped
	tagsHash uint32
	resource *resourcev1.Resource
}

func jSpanToOTLP(jSpan *model.Span) (*tracev1.Span, resource, instrumentationLibrary) {
	tags := model.KeyValues(jSpan.GetTags())
	status, ignoreKeys := getSpanStatus(tags)

	traceState := getTraceStateFromAttrs(tags)
	if traceState != "" {
		ignoreKeys[tagW3CTraceState] = true
	}

	s := &tracev1.Span{
		TraceId:           uint64ToTraceID(jSpan.TraceID.High, jSpan.TraceID.Low),
		SpanId:            uint64ToSpanID(uint64(jSpan.SpanID)),
		ParentSpanId:      uint64ToSpanID(uint64(jSpan.ParentSpanID())),
		TraceState:        traceState,
		Name:              jSpan.GetOperationName(),
		StartTimeUnixNano: uint64(jSpan.GetStartTime().UnixNano()),
		EndTimeUnixNano:   uint64(jSpan.GetStartTime().Add(jSpan.GetDuration()).UnixNano()),
		Events:            jLogsToOTLP(jSpan.GetLogs()),
		Links:             jReferencesToOTLP(jSpan.GetReferences(), jSpan.ParentSpanID()),
		Status:            status,
		Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
	}
	if kind, found := jSpan.GetSpanKind(); found {
		s.Kind = jSpanKindToInternal(kind)
		ignoreKeys[tagSpanKind] = true
	}

	il := instrumentationLibrary{}
	if libraryName, ok := tags.FindByKey(semconv.InstrumentationLibraryName); ok {
		il.name = libraryName.GetVStr()
		ignoreKeys[semconv.InstrumentationLibraryName] = true
		if libraryVersion, ok := tags.FindByKey(semconv.InstrumentationLibraryVersion); ok {
			il.version = libraryVersion.GetVStr()
			ignoreKeys[semconv.InstrumentationLibraryVersion] = true
		}
	}
	// convert to attributes at the end once not needed attrs are removed
	attrs := jTagsToOTLP(tags, ignoreKeys)
	s.Attributes = attrs

	res := resource{}
	if jSpan.GetProcess() != nil {
		tags := concatStringTags(jSpan.GetProcess().GetTags())
		fnva := fnv.New32a()
		fnva.Write([]byte(tags))

		res.serviceName = jSpan.GetProcess().GetServiceName()
		res.tagsHash = fnva.Sum32()
		res.resource = jProcessToInternalResource(jSpan.GetProcess())
	}
	return s, res, il
}

// concatStringTags returns key sorted concatenated string tags
// e.g. keyB=val1,keyA=val2 becomes keyAval2KeyBval1
func concatStringTags(tags []model.KeyValue) string {
	keys := make([]string, len(tags))
	tagMap := make(map[string]string, len(tags))
	stringTagsLen := 0
	for i, t := range tags {
		if t.GetVType() == model.ValueType_STRING {
			keys[i] = t.GetKey()
			tagMap[t.GetKey()] = t.GetVStr()
			stringTagsLen += len(t.GetKey()) + len(t.GetVStr())
		}
	}
	sort.Strings(keys)
	sBuilder := strings.Builder{}
	sBuilder.Grow(stringTagsLen)
	for k, v := range tagMap {
		sBuilder.WriteString(k)
		sBuilder.WriteString(v)
	}
	return sBuilder.String()
}

func jProcessToInternalResource(process *model.Process) *resourcev1.Resource {
	if process == nil {
		return nil
	}
	tags := process.GetTags()
	if process.GetServiceName() != "" {
		tags = append(tags, model.String(semconv.AttributeServiceName, process.GetServiceName()))
	}
	return &resourcev1.Resource{
		Attributes: jTagsToOTLP(tags, nil),
	}
}

func jTagsToOTLP(tags []model.KeyValue, ignoreKeys map[string]bool) []*commonv1.KeyValue {
	var kvs []*commonv1.KeyValue
	for _, tag := range tags {
		if ignoreKeys[tag.GetKey()] {
			continue
		}

		kv := &commonv1.KeyValue{
			Key:   tag.GetKey(),
			Value: &commonv1.AnyValue{},
		}
		switch tag.GetVType() {
		case model.ValueType_STRING:
			kv.Value.Value = &commonv1.AnyValue_StringValue{
				StringValue: tag.GetVStr(),
			}
		case model.ValueType_BOOL:
			kv.Value.Value = &commonv1.AnyValue_BoolValue{
				BoolValue: tag.GetVBool(),
			}
		case model.ValueType_INT64:
			kv.Value.Value = &commonv1.AnyValue_IntValue{
				IntValue: tag.GetVInt64(),
			}
		case model.ValueType_FLOAT64:
			kv.Value.Value = &commonv1.AnyValue_DoubleValue{
				DoubleValue: tag.GetVFloat64(),
			}
		case model.ValueType_BINARY:
			kv.Value.Value = &commonv1.AnyValue_BytesValue{
				BytesValue: tag.GetVBinary(),
			}
		default:
			kv.Value.Value = &commonv1.AnyValue_StringValue{
				StringValue: tag.String(),
			}
		}
		kvs = append(kvs, kv)
	}
	return kvs
}

func jLogsToOTLP(logs []model.Log) []*tracev1.Span_Event {
	events := make([]*tracev1.Span_Event, len(logs))
	for i, l := range logs {

		var name string
		var ignoreKeys map[string]bool
		if messageTag, ok := model.KeyValues(l.GetFields()).FindByKey(tagMessage); ok {
			name = messageTag.GetVStr()
			ignoreKeys = map[string]bool{}
			ignoreKeys[tagMessage] = true
		}

		events[i] = &tracev1.Span_Event{
			TimeUnixNano: uint64(l.GetTimestamp().UnixNano()),
			Name:         name,
			Attributes:   jTagsToOTLP(l.GetFields(), ignoreKeys),
		}
	}
	return events
}

func jReferencesToOTLP(refs []model.SpanRef, excludeParentID model.SpanID) []*tracev1.Span_Link {
	if len(refs) == 0 || len(refs) == 1 && refs[0].SpanID == excludeParentID && refs[0].RefType == model.ChildOf {
		return nil
	}
	var links []*tracev1.Span_Link
	for _, r := range refs {
		if r.SpanID == excludeParentID && r.GetRefType() == model.ChildOf {
			continue
		}
		links = append(links, &tracev1.Span_Link{
			TraceId: uint64ToTraceID(r.TraceID.High, r.TraceID.Low),
			SpanId:  uint64ToSpanID(uint64(r.SpanID)),
		})
	}

	return links
}

func getSpanStatus(tags []model.KeyValue) (*tracev1.Status, map[string]bool) {
	statusCode := tracev1.Status_STATUS_CODE_UNSET
	statusMessage := ""
	statusExists := false

	ignoreKeys := map[string]bool{}
	kvs := model.KeyValues(tags)
	if _, ok := kvs.FindByKey(tagError); ok {
		statusCode = tracev1.Status_STATUS_CODE_ERROR
		statusExists = true
		ignoreKeys[tagError] = true
	}
	if tag, ok := kvs.FindByKey(tagStatusCode); ok {
		statusExists = true
		if code, err := getStatusCodeValFromTag(tag); err == nil {
			statusCode = tracev1.Status_StatusCode(code)
			ignoreKeys[tagStatusCode] = true
		}
		if tag, ok := kvs.FindByKey(tagStatusMsg); ok {
			statusMessage = tag.GetVStr()
			ignoreKeys[tagStatusMsg] = true
		}
	} else if tag, ok := kvs.FindByKey(semconv.AttributeHTTPStatusCode); ok {
		statusExists = true
		if code, err := getStatusCodeFromHTTPStatusTag(tag); err == nil {
			// Do not set status code in case it was set to Unset.
			if tracev1.Status_StatusCode(code) != tracev1.Status_STATUS_CODE_UNSET {
				statusCode = tracev1.Status_StatusCode(code)
			}

			if tag, ok := kvs.FindByKey(tagHTTPStatusMsg); ok {
				statusMessage = tag.GetVStr()
			}
		}
	}

	if statusExists {
		return &tracev1.Status{
			Code:    statusCode,
			Message: statusMessage,
		}, ignoreKeys
	}
	return nil, ignoreKeys
}

func getStatusCodeValFromTag(tag model.KeyValue) (int, error) {
	var codeVal int64
	switch tag.GetVType() {
	case model.ValueType_INT64:
		codeVal = tag.GetVInt64()
	case model.ValueType_STRING:
		i, err := strconv.Atoi(tag.GetVStr())
		if err != nil {
			return 0, err
		}
		codeVal = int64(i)
	default:
		return 0, fmt.Errorf("invalid status code attribute type: %q, key: %q", tag.GetKey(), tag.GetKey())
	}
	if codeVal > math.MaxInt32 || codeVal < math.MinInt32 {
		return 0, fmt.Errorf("invalid status code value: %d", codeVal)
	}
	return int(codeVal), nil
}

func getStatusCodeFromHTTPStatusTag(tag model.KeyValue) (int, error) {
	statusCode, err := getStatusCodeValFromTag(tag)
	if err != nil {
		return int(tracev1.Status_STATUS_CODE_OK), err
	}

	return int(statusCodeFromHTTP(statusCode)), nil
}

func jSpanKindToInternal(spanKind string) tracev1.Span_SpanKind {
	switch spanKind {
	case "client":
		return tracev1.Span_SPAN_KIND_CLIENT
	case "server":
		return tracev1.Span_SPAN_KIND_SERVER
	case "producer":
		return tracev1.Span_SPAN_KIND_PRODUCER
	case "consumer":
		return tracev1.Span_SPAN_KIND_CONSUMER
	case "internal":
		return tracev1.Span_SPAN_KIND_INTERNAL
	}
	return tracev1.Span_SPAN_KIND_UNSPECIFIED
}

func getTraceStateFromAttrs(attrs []model.KeyValue) string {
	traceState := ""
	for _, attr := range attrs {
		if attr.GetKey() == tagW3CTraceState {
			return attr.GetVStr()
		}
	}
	return traceState
}

func uint64ToSpanID(id uint64) []byte {
	spanID := [8]byte{}
	binary.BigEndian.PutUint64(spanID[:], id)
	return spanID[:]
}

func uint64ToTraceID(high, low uint64) []byte {
	traceID := [16]byte{}
	binary.BigEndian.PutUint64(traceID[:8], high)
	binary.BigEndian.PutUint64(traceID[8:], low)
	return traceID[:]
}

// statusCodeFromHTTP takes an HTTP status code and return the appropriate OpenTelemetry status code
// See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/http.md#status
func statusCodeFromHTTP(httpStatusCode int) pdata.StatusCode {
	if httpStatusCode >= 100 && httpStatusCode < 399 {
		return ptrace.StatusCodeUnset
	}
	return ptrace.StatusCodeError
}

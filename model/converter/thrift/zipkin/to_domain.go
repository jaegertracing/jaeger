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

package zipkin

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/opentracing/opentracing-go/ext"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/multierror"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

const (
	// UnknownServiceName is serviceName we give to model.Proces if we cannot find it anywhere in a Zipkin span
	UnknownServiceName = "unknown-service-name"
)

var (
	coreAnnotations = map[string]string{
		zipkincore.SERVER_RECV: string(ext.SpanKindRPCServerEnum),
		zipkincore.SERVER_SEND: string(ext.SpanKindRPCServerEnum),
		zipkincore.CLIENT_RECV: string(ext.SpanKindRPCClientEnum),
		zipkincore.CLIENT_SEND: string(ext.SpanKindRPCClientEnum),
	}

	// Some tags on Zipkin spans really describe the process emitting them rather than an individual span.
	// Once all clients are upgraded to use native Jaeger model, this won't be happenning, but for now
	// we remove these tags from the span and store them in the Process.
	processTagAnnotations = map[string]string{
		"jaegerClient":    "jaeger.version", // transform this tag name to client.version
		"jaeger.hostname": "hostname",       // transform this tag name to hostname
		"jaeger.version":  "jaeger.version", // keep this key as is
	}

	trueByteSlice = []byte{1}

	// DefaultLogFieldKey is the log field key which translates directly into Zipkin's Annotation.Value,
	// provided it's the only field in the log.
	// In all other cases the fields are encoded into Annotation.Value as JSON string.
	// TODO move to domain model
	DefaultLogFieldKey = "event"

	// IPTagName is the Jaeger tag name for an IPv4/IPv6 IP address.
	// TODO move to domain model
	IPTagName = "ip"
)

// ToDomain transforms a trace in zipkin.thrift format into model.Trace.
// The transformation assumes that all spans have the same Trace ID.
// A valid model.Trace is always returned, even when there are errors.
// The errors are more of an "fyi", describing issues in the data.
// TODO consider using different return type instead of `error`.
func ToDomain(zSpans []*zipkincore.Span) (*model.Trace, error) {
	return toDomain{}.ToDomain(zSpans)
}

// ToDomainSpan transforms a span in zipkin.thrift format into model.Span.
// A valid model.Span is always returned, even when there are errors.
// The errors are more of an "fyi", describing issues in the data.
// TODO consider using different return type instead of `error`.
func ToDomainSpan(zSpan *zipkincore.Span) (*model.Span, error) {
	return toDomain{}.ToDomainSpan(zSpan)
}

type toDomain struct{}

func (td toDomain) ToDomain(zSpans []*zipkincore.Span) (*model.Trace, error) {
	var errors []error
	trace := &model.Trace{}
	processes := newProcessHashtable()
	for _, zSpan := range zSpans {
		jSpan, err := td.ToDomainSpan(zSpan)
		if err != nil {
			errors = append(errors, err)
		}
		// remove duplicate Process instances
		jSpan.Process = processes.add(jSpan.Process)
		trace.Spans = append(trace.Spans, jSpan)
	}
	return trace, multierror.Wrap(errors)
}

func (td toDomain) ToDomainSpan(zSpan *zipkincore.Span) (*model.Span, error) {
	jSpan := td.transformSpan(zSpan)
	jProcess, err := td.generateProcess(zSpan)
	jSpan.Process = jProcess
	return jSpan, err
}

// transformSpan transforms a zipkin span into a Jaeger span
func (td toDomain) transformSpan(zSpan *zipkincore.Span) *model.Span {
	tags := td.getTags(zSpan.BinaryAnnotations, td.isSpanTag)
	if spanKindTag, ok := td.getSpanKindTag(zSpan.Annotations); ok {
		tags = append(tags, spanKindTag)
	}
	var parentID int64
	if zSpan.ParentID != nil {
		parentID = *zSpan.ParentID
	}
	span := &model.Span{
		TraceID:       model.TraceID{Low: uint64(zSpan.TraceID)},
		SpanID:        model.SpanID(zSpan.ID),
		OperationName: zSpan.Name,
		ParentSpanID:  model.SpanID(parentID),
		Flags:         td.getFlags(zSpan),
		StartTime:     model.EpochMicrosecondsAsTime(uint64(zSpan.GetTimestamp())),
		Duration:      model.MicrosecondsAsDuration(uint64(zSpan.GetDuration())),
		Tags:          tags,
		Logs:          td.getLogs(zSpan.Annotations),
	}

	if zSpan.TraceIDHigh != nil {
		span.TraceID.High = uint64(*zSpan.TraceIDHigh)
	}

	return span
}

// getFlags takes a Zipkin Span and deduces the proper flags settings
func (td toDomain) getFlags(zSpan *zipkincore.Span) model.Flags {
	f := model.Flags(0)
	if zSpan.Debug {
		f.SetDebug()
	}
	return f
}

// generateProcess takes a Zipkin Span and produces a model.Process.
// An optional error may also be returned, but it is not fatal.
func (td toDomain) generateProcess(zSpan *zipkincore.Span) (*model.Process, error) {
	tags := td.getTags(zSpan.BinaryAnnotations, td.isProcessTag)
	for i, tag := range tags {
		tags[i].Key = processTagAnnotations[tag.Key]
	}
	serviceName, ipv4, err := td.findServiceNameAndIP(zSpan)
	if ipv4 != 0 {
		// If the ip process tag already exists, don't add it again
		tags = append(tags, model.Int64(IPTagName, int64(uint64(ipv4))))
	}
	return model.NewProcess(serviceName, tags), err
}

func (td toDomain) findServiceNameAndIP(zSpan *zipkincore.Span) (string, int32, error) {
	for _, a := range zSpan.Annotations {
		if td.isCoreAnnotation(a) && a.Host != nil && a.Host.ServiceName != "" {
			return a.Host.ServiceName, a.Host.Ipv4, nil
		}
	}
	for _, a := range zSpan.BinaryAnnotations {
		if a.Key == zipkincore.LOCAL_COMPONENT && a.Host != nil && a.Host.ServiceName != "" {
			return a.Host.ServiceName, a.Host.Ipv4, nil
		}
	}
	// If no core annotations exist, use the service name from any annotation
	for _, a := range zSpan.Annotations {
		if a.Host != nil && a.Host.ServiceName != "" {
			return a.Host.ServiceName, a.Host.Ipv4, nil
		}
	}
	err := fmt.Errorf(
		"Cannot find service name in Zipkin span [traceID=%x, spanID=%x]",
		uint64(zSpan.TraceID), uint64(zSpan.ID))
	return UnknownServiceName, 0, err
}

func (td toDomain) isCoreAnnotation(annotation *zipkincore.Annotation) bool {
	_, ok := coreAnnotations[annotation.Value]
	return ok
}

func (td toDomain) isProcessTag(binaryAnnotation *zipkincore.BinaryAnnotation) bool {
	_, ok := processTagAnnotations[binaryAnnotation.Key]
	return ok
}

func (td toDomain) isSpanTag(binaryAnnotation *zipkincore.BinaryAnnotation) bool {
	return !td.isProcessTag(binaryAnnotation)
}

type tagPredicate func(*zipkincore.BinaryAnnotation) bool

func (td toDomain) getTags(binAnnotations []*zipkincore.BinaryAnnotation, tagInclude tagPredicate) []model.KeyValue {
	// this will be memory intensive due to how slices work, and it's specifically because we have to filter out
	// some binary annotations. improvement here would be just collecting the indices in binAnnotations we want.
	var retMe []model.KeyValue
	for _, annotation := range binAnnotations {
		if !tagInclude(annotation) {
			continue
		}
		switch annotation.Key {
		case zipkincore.LOCAL_COMPONENT:
			value := string(annotation.Value)
			tag := model.String(string(ext.Component), value)
			retMe = append(retMe, tag)
		case zipkincore.SERVER_ADDR, zipkincore.CLIENT_ADDR:
			retMe = td.getPeerTags(annotation.Host, retMe)
		default:
			tag, err := td.transformBinaryAnnotation(annotation)
			if err != nil {
				encoded := base64.StdEncoding.EncodeToString(annotation.Value)
				errMsg := fmt.Sprintf("Cannot parse Zipkin value %s: %v", encoded, err)
				tag = model.String(annotation.Key, errMsg)
			}
			retMe = append(retMe, tag)
		}
	}
	return retMe
}

func (td toDomain) transformBinaryAnnotation(binaryAnnotation *zipkincore.BinaryAnnotation) (model.KeyValue, error) {
	switch binaryAnnotation.AnnotationType {
	case zipkincore.AnnotationType_BOOL:
		vBool := bytes.Compare(binaryAnnotation.Value, trueByteSlice) == 0
		return model.Bool(binaryAnnotation.Key, vBool), nil
	case zipkincore.AnnotationType_BYTES:
		return model.Binary(binaryAnnotation.Key, binaryAnnotation.Value), nil
	case zipkincore.AnnotationType_DOUBLE:
		var d float64
		if err := bytesToNumber(binaryAnnotation.Value, &d); err != nil {
			return model.KeyValue{}, err
		}
		return model.Float64(binaryAnnotation.Key, d), nil
	case zipkincore.AnnotationType_I16:
		var i int16
		if err := bytesToNumber(binaryAnnotation.Value, &i); err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(binaryAnnotation.Key, int64(i)), nil
	case zipkincore.AnnotationType_I32:
		var i int32
		if err := bytesToNumber(binaryAnnotation.Value, &i); err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(binaryAnnotation.Key, int64(i)), nil
	case zipkincore.AnnotationType_I64:
		var i int64
		if err := bytesToNumber(binaryAnnotation.Value, &i); err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(binaryAnnotation.Key, int64(i)), nil
	case zipkincore.AnnotationType_STRING:
		return model.String(binaryAnnotation.Key, string(binaryAnnotation.Value)), nil
	}
	return model.KeyValue{}, fmt.Errorf("Unknown zipkin annotation type: %d", binaryAnnotation.AnnotationType)
}

func bytesToNumber(b []byte, number interface{}) error {
	buf := bytes.NewReader(b)
	if err := binary.Read(buf, binary.BigEndian, number); err != nil {
		return err
	}
	return nil
}

func (td toDomain) getLogs(annotations []*zipkincore.Annotation) []model.Log {
	var retMe []model.Log
	for _, a := range annotations {
		// If the annotation has no value, throw it out
		if a.Value == "" {
			continue
		}
		if _, ok := coreAnnotations[a.Value]; ok {
			// skip core annotations
			continue
		}
		logFields := td.getLogFields(a)
		jLog := model.Log{
			Timestamp: model.EpochMicrosecondsAsTime(uint64(a.Timestamp)),
			Fields:    logFields,
		}
		retMe = append(retMe, jLog)
	}
	return retMe
}

func (td toDomain) getLogFields(annotation *zipkincore.Annotation) []model.KeyValue {
	var logFields map[string]string
	// Since Zipkin format does not support kv-logging, some clients encode those Logs
	// as annotations with JSON value. Therefore, we try JSON decoding first.
	if err := json.Unmarshal([]byte(annotation.Value), &logFields); err == nil {
		fields := make([]model.KeyValue, len(logFields))
		i := 0
		for k, v := range logFields {
			fields[i] = model.String(k, v)
			i++
		}
		return fields
	}
	return []model.KeyValue{model.String(DefaultLogFieldKey, annotation.Value)}
}

func (td toDomain) getSpanKindTag(annotations []*zipkincore.Annotation) (model.KeyValue, bool) {
	for _, a := range annotations {
		if spanKind, ok := coreAnnotations[a.Value]; ok {
			return model.String(string(ext.SpanKind), spanKind), true
		}
	}
	return model.KeyValue{}, false
}

func (td toDomain) getPeerTags(endpoint *zipkincore.Endpoint, tags []model.KeyValue) []model.KeyValue {
	if endpoint == nil {
		return tags
	}
	tags = append(tags, model.String(string(ext.PeerService), endpoint.ServiceName))
	if endpoint.Ipv4 != 0 {
		ipv4 := int64(uint32(endpoint.Ipv4))
		tags = append(tags, model.Int64(string(ext.PeerHostIPv4), ipv4))
	}
	if endpoint.Ipv6 != nil {
		ipv6 := string(endpoint.Ipv6)
		tags = append(tags, model.String(string(ext.PeerHostIPv6), ipv6))
	}
	if endpoint.Port != 0 {
		port := int64(uint16(endpoint.Port))
		tags = append(tags, model.Int64(string(ext.PeerPort), port))
	}
	return tags
}

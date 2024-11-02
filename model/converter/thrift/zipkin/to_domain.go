// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package zipkin

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	// UnknownServiceName is serviceName we give to model.Process if we cannot find it anywhere in a Zipkin span
	UnknownServiceName = "unknown-service-name"
	keySpanKind        = "span.kind"
	component          = "component"
	peerservice        = "peer.service"
	peerHostIPv4       = "peer.ipv4"
	peerHostIPv6       = "peer.ipv6"
	peerPort           = "peer.port"
)

var (
	coreAnnotations = map[string]string{
		zipkincore.SERVER_RECV: trace.SpanKindServer.String(),
		zipkincore.SERVER_SEND: trace.SpanKindServer.String(),
		zipkincore.CLIENT_RECV: trace.SpanKindClient.String(),
		zipkincore.CLIENT_SEND: trace.SpanKindClient.String(),
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
func ToDomainSpan(zSpan *zipkincore.Span) ([]*model.Span, error) {
	return toDomain{}.ToDomainSpans(zSpan)
}

type toDomain struct{}

func (td toDomain) ToDomain(zSpans []*zipkincore.Span) (*model.Trace, error) {
	var errs []error
	processes := newProcessHashtable()
	trace := &model.Trace{}
	for _, zSpan := range zSpans {
		jSpans, err := td.ToDomainSpans(zSpan)
		if err != nil {
			errs = append(errs, err)
		}
		for _, jSpan := range jSpans {
			// remove duplicate Process instances
			jSpan.Process = processes.add(jSpan.Process)
			trace.Spans = append(trace.Spans, jSpan)
		}
	}
	return trace, errors.Join(errs...)
}

func (td toDomain) ToDomainSpans(zSpan *zipkincore.Span) ([]*model.Span, error) {
	jSpans := td.transformSpan(zSpan)
	jProcess, err := td.generateProcess(zSpan)
	for _, jSpan := range jSpans {
		jSpan.Process = jProcess
	}
	return jSpans, err
}

func (toDomain) findAnnotation(zSpan *zipkincore.Span, value string) *zipkincore.Annotation {
	for _, ann := range zSpan.Annotations {
		if ann.Value == value {
			return ann
		}
	}
	return nil
}

// transformSpan transforms a zipkin span into a Jaeger span
func (td toDomain) transformSpan(zSpan *zipkincore.Span) []*model.Span {
	tags := td.getTags(zSpan.BinaryAnnotations, td.isSpanTag)
	if spanKindTag, ok := td.getSpanKindTag(zSpan.Annotations); ok {
		tags = append(tags, spanKindTag)
	}
	var traceIDHigh int64
	if zSpan.TraceIDHigh != nil {
		traceIDHigh = *zSpan.TraceIDHigh
	}
	//nolint: gosec // G115
	traceID := model.NewTraceID(uint64(traceIDHigh), uint64(zSpan.TraceID))
	var refs []model.SpanRef
	if zSpan.ParentID != nil {
		//nolint: gosec // G115
		parentSpanID := model.NewSpanID(uint64(*zSpan.ParentID))
		refs = model.MaybeAddParentSpanID(traceID, parentSpanID, refs)
	}

	flags := td.getFlags(zSpan)

	startTime, duration := td.getStartTimeAndDuration(zSpan)

	result := []*model.Span{{
		TraceID: traceID,
		//nolint: gosec // G115
		SpanID:        model.NewSpanID(uint64(zSpan.ID)),
		OperationName: zSpan.Name,
		References:    refs,
		Flags:         flags,
		//nolint: gosec // G115
		StartTime: model.EpochMicrosecondsAsTime(uint64(startTime)),
		//nolint: gosec // G115
		Duration: model.MicrosecondsAsDuration(uint64(duration)),
		Tags:     tags,
		Logs:     td.getLogs(zSpan.Annotations),
	}}

	cs := td.findAnnotation(zSpan, zipkincore.CLIENT_SEND)
	sr := td.findAnnotation(zSpan, zipkincore.SERVER_RECV)
	if cs != nil && sr != nil {
		// if the span is client and server we split it into two separate spans
		s := &model.Span{
			TraceID: traceID,
			//nolint: gosec // G115
			SpanID:        model.NewSpanID(uint64(zSpan.ID)),
			OperationName: zSpan.Name,
			References:    refs,
			Flags:         flags,
		}
		// if the first span is a client span we create server span and vice-versa.
		if result[0].IsRPCClient() {
			s.Tags = []model.KeyValue{model.String(keySpanKind, trace.SpanKindServer.String())}
			//nolint: gosec // G115
			s.StartTime = model.EpochMicrosecondsAsTime(uint64(sr.Timestamp))
			if ss := td.findAnnotation(zSpan, zipkincore.SERVER_SEND); ss != nil {
				//nolint: gosec // G115
				s.Duration = model.MicrosecondsAsDuration(uint64(ss.Timestamp - sr.Timestamp))
			}
		} else {
			s.Tags = []model.KeyValue{model.String(keySpanKind, trace.SpanKindClient.String())}
			//nolint: gosec // G115
			s.StartTime = model.EpochMicrosecondsAsTime(uint64(cs.Timestamp))
			if cr := td.findAnnotation(zSpan, zipkincore.CLIENT_RECV); cr != nil {
				//nolint: gosec // G115
				s.Duration = model.MicrosecondsAsDuration(uint64(cr.Timestamp - cs.Timestamp))
			}
		}
		result = append(result, s)
	}
	return result
}

// getFlags takes a Zipkin Span and deduces the proper flags settings
func (toDomain) getFlags(zSpan *zipkincore.Span) model.Flags {
	f := model.Flags(0)
	if zSpan.Debug {
		f.SetDebug()
	}
	return f
}

// Get a correct start time to use for the span if it's not set directly
func (td toDomain) getStartTimeAndDuration(zSpan *zipkincore.Span) (timestamp, duration int64) {
	timestamp = zSpan.GetTimestamp()
	duration = zSpan.GetDuration()
	if timestamp == 0 {
		cs := td.findAnnotation(zSpan, zipkincore.CLIENT_SEND)
		sr := td.findAnnotation(zSpan, zipkincore.SERVER_RECV)
		if cs != nil {
			timestamp = cs.Timestamp
			cr := td.findAnnotation(zSpan, zipkincore.CLIENT_RECV)
			if cr != nil && duration == 0 {
				duration = cr.Timestamp - cs.Timestamp
			}
		} else if sr != nil {
			timestamp = sr.Timestamp
			ss := td.findAnnotation(zSpan, zipkincore.SERVER_SEND)
			if ss != nil && duration == 0 {
				duration = ss.Timestamp - sr.Timestamp
			}
		}
	}
	return timestamp, duration
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
		//nolint: gosec // G115
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
	// Tracer can also report a span with just binary annotation/s
	for _, a := range zSpan.BinaryAnnotations {
		if a.Host != nil && a.Host.ServiceName != "" {
			return a.Host.ServiceName, a.Host.Ipv4, nil
		}
	}
	err := fmt.Errorf(
		"cannot find service name in Zipkin span [traceID=%x, spanID=%x]",
		//nolint: gosec // G115
		uint64(zSpan.TraceID), uint64(zSpan.ID))
	return UnknownServiceName, 0, err
}

func (toDomain) isCoreAnnotation(annotation *zipkincore.Annotation) bool {
	_, ok := coreAnnotations[annotation.Value]
	return ok
}

func (toDomain) isProcessTag(binaryAnnotation *zipkincore.BinaryAnnotation) bool {
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
			tag := model.String(component, value)
			retMe = append(retMe, tag)
		case zipkincore.SERVER_ADDR, zipkincore.CLIENT_ADDR, zipkincore.MESSAGE_ADDR:
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

func (toDomain) transformBinaryAnnotation(binaryAnnotation *zipkincore.BinaryAnnotation) (model.KeyValue, error) {
	switch binaryAnnotation.AnnotationType {
	case zipkincore.AnnotationType_BOOL:
		vBool := bytes.Equal(binaryAnnotation.Value, trueByteSlice)
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
	return model.KeyValue{}, fmt.Errorf("unknown zipkin annotation type: %d", binaryAnnotation.AnnotationType)
}

func bytesToNumber(b []byte, number any) error {
	buf := bytes.NewReader(b)
	return binary.Read(buf, binary.BigEndian, number)
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
			//nolint: gosec // G115
			Timestamp: model.EpochMicrosecondsAsTime(uint64(a.Timestamp)),
			Fields:    logFields,
		}
		retMe = append(retMe, jLog)
	}
	return retMe
}

func (toDomain) getLogFields(annotation *zipkincore.Annotation) []model.KeyValue {
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

func (toDomain) getSpanKindTag(annotations []*zipkincore.Annotation) (model.KeyValue, bool) {
	for _, a := range annotations {
		if spanKind, ok := coreAnnotations[a.Value]; ok {
			return model.String(keySpanKind, spanKind), true
		}
	}
	return model.KeyValue{}, false
}

func (toDomain) getPeerTags(endpoint *zipkincore.Endpoint, tags []model.KeyValue) []model.KeyValue {
	if endpoint == nil {
		return tags
	}
	tags = append(tags, model.String(peerservice, endpoint.ServiceName))
	if endpoint.Ipv4 != 0 {
		//nolint: gosec // G115
		ipv4 := int64(uint32(endpoint.Ipv4))
		tags = append(tags, model.Int64(peerHostIPv4, ipv4))
	}
	if endpoint.Ipv6 != nil {
		// Zipkin defines Ipv6 field as: "IPv6 host address packed into 16 bytes. Ex Inet6Address.getBytes()".
		// https://github.com/openzipkin/zipkin-api/blob/master/thrift/zipkinCore.thrift#L305
		tags = append(tags, model.Binary(peerHostIPv6, endpoint.Ipv6))
	}
	if endpoint.Port != 0 {
		//nolint: gosec // G115
		port := int64(uint16(endpoint.Port))
		tags = append(tags, model.Int64(peerPort, port))
	}
	return tags
}

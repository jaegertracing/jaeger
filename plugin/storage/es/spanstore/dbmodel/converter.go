package dbmodel

import (
	"strconv"
	"errors"
	"encoding/hex"

	"github.com/uber/jaeger/model"
)

var ErrUnknownKeyValueTypeFromES = errors.New("Unknown tag type found in ES")

// FromDomain converts a domain model.Span to a database Span
func FromDomain(span *model.Span) *Span {
	return converter{}.fromDomain(span)
}

// ToDomain converts a database Span to a domain model.Span
func ToDomain(dbSpan *Span) (*model.Span, error) {
	return converter{}.toDomain(dbSpan)
}

// converter converts Spans between domain and database representations.
// It primarily exists to namespace the conversion functions.
type converter struct{}

func (c converter) fromDomain(span *model.Span) *Span {
	tags := c.toDBTags(span.Tags)
	logs := c.toDBLogs(span.Logs)
	refs := c.toDBRefs(span.References)
	udtProcess := c.toDBProcess(span.Process)


	return &Span{
		TraceID: 	TraceIDFromDomain(span.TraceID),
		ParentSpanID:	SpanID(uint64(span.ParentSpanID)),
		SpanID: 	SpanID(uint64(span.SpanID)),
		Flags: 		uint32(span.Flags),
		OperationName: 	span.OperationName,
		References:	refs,
		Timestamp:	uint64(model.TimeAsEpochMicroseconds(span.StartTime)),
		Duration:	uint64(model.DurationAsMicroseconds(span.Duration)),
		Tags:		tags,
		Logs:		logs,
		Process:	udtProcess,
	}
}

func (c converter) toDomain(dbSpan *Span) (*model.Span, error) {
	tags, err := c.fromDBTags(dbSpan.Tags)
	if err != nil {
		return nil, err
	}
	logs, err := c.fromDBLogs(dbSpan.Logs)
	if err != nil {
		return nil, err
	}
	refs, err := c.fromDBRefs(dbSpan.References)
	if err != nil {
		return nil, err
	}
	process, err := c.fromDBProcess(dbSpan.Process)
	if err != nil {
		return nil, err
	}
	traceID, err := dbSpan.TraceID.TraceIDToDomain()
	if err != nil {
		return nil, err
	}

	span := &model.Span{
		TraceID:       traceID,
		SpanID:        model.SpanID(dbSpan.SpanID),
		ParentSpanID:  model.SpanID(dbSpan.ParentSpanID),
		OperationName: dbSpan.OperationName,
		References:    refs,
		Flags:         model.Flags(uint32(dbSpan.Flags)),
		StartTime:     model.EpochMicrosecondsAsTime(uint64(dbSpan.Timestamp)),
		Duration:      model.MicrosecondsAsDuration(uint64(dbSpan.Duration)),
		Tags:          tags,
		Logs:          logs,
		Process:       process,
	}
	return span, nil
}


func (c converter) fromDBTags(tags []Tag) ([]model.KeyValue, error) {
	retMe := make([]model.KeyValue, len(tags))
	for i := range tags {
		kv, err := c.fromDBTag(&tags[i])
		if err != nil {
			return nil, err
		}
		retMe[i] = kv
	}
	return retMe, nil
}

func (c converter) fromDBTag(tag *Tag) (model.KeyValue, error) {
	vType, err := model.ValueTypeFromString(tag.TagType)
	if err != nil {
		return model.KeyValue{}, err
	}
	return c.fromDBTagOfType(tag, vType)
}

func (c converter) fromDBTagOfType(tag *Tag, vType model.ValueType) (model.KeyValue, error) {
	switch vType {
	case model.StringType:
		return model.String(tag.Key, tag.Value), nil
	case model.BoolType:
		value, err := strconv.ParseBool(tag.Value)
		if err != nil {
			return model.KeyValue{}, ErrUnknownKeyValueTypeFromES
		}
		return model.Bool(tag.Key, value), nil
	case model.Int64Type:
		value, err := strconv.ParseInt(tag.Value, 10, 64)
		if err != nil {
			return model.KeyValue{}, ErrUnknownKeyValueTypeFromES
		}
		return model.Int64(tag.Key, value), nil
	case model.Float64Type:
		value, err := strconv.ParseFloat(tag.Value, 64)
		if err != nil {
			return model.KeyValue{}, ErrUnknownKeyValueTypeFromES
		}
		return model.Float64(tag.Key, value), nil
	case model.BinaryType:
		value, err := hex.DecodeString(tag.Value)
		if err != nil {
			return model.KeyValue{}, ErrUnknownKeyValueTypeFromES
		}
		return model.Binary(tag.Key, value), nil
	}
	return model.KeyValue{}, ErrUnknownKeyValueTypeFromES
}

func (c converter) fromDBLogs(logs []Log) ([]model.Log, error) {
	retMe := make([]model.Log, len(logs))
	for i, l := range logs {
		fields, err := c.fromDBTags(l.Tags)
		if err != nil {
			return nil, err
		}
		retMe[i] = model.Log{
			Timestamp: model.EpochMicrosecondsAsTime(uint64(l.Timestamp)),
			Fields:    fields,
		}
	}
	return retMe, nil
}

func (c converter) fromDBRefs(refs []Reference) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		refType, err := model.SpanRefTypeFromString(r.RefType)
		if err != nil {
			return nil, err
		}
		traceID, err := r.TraceID.TraceIDToDomain()
		if err != nil {
			return nil, err
		}

		retMe[i] = model.SpanRef{
			RefType: refType,
			TraceID: traceID,
			SpanID:  model.SpanID(r.SpanID),
		}
	}
	return retMe, nil
}

func (c converter) fromDBProcess(process Process) (*model.Process, error) {
	tags, err := c.fromDBTags(process.Tags)
	if err != nil {
		return nil, err
	}
	return &model.Process{
		Tags:        tags,
		ServiceName: process.ServiceName,
	}, nil
}



func (c converter) toDBTags(tags []model.KeyValue) []Tag {
	retMe := make([]Tag, len(tags))
	for i, t := range tags {
		retMe[i] = Tag{
			Key: t.Key,
			Value: t.AsString(),
			TagType: t.VType.String(),
		}
	}
	return retMe
}

func (c converter) toDBLogs(logs []model.Log) []Log {
	retMe := make([]Log, len(logs))
	for i, l := range logs {
		retMe[i] = Log{
			Timestamp: uint64(model.TimeAsEpochMicroseconds(l.Timestamp)),
			Tags:    c.toDBTags(l.Fields),
		}
	}
	return retMe
}

func (c converter) toDBRefs(refs []model.SpanRef) []Reference {
	retMe := make([]Reference, len(refs))
	for i, r := range refs {
		retMe[i] = Reference {
			TraceID: TraceIDFromDomain(r.TraceID),
			SpanID:  SpanID(int64(r.SpanID)),
			RefType: r.RefType.String(),
		}
	}
	return retMe
}

func (c converter) toDBProcess(process *model.Process) Process {
	return Process{
		ServiceName: process.ServiceName,
		Tags:        c.toDBTags(process.Tags),
	}
}

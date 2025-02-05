// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

type badgerSpanKind byte

const (
	badgerSpanKindUnspecified badgerSpanKind = 0x86
	badgerSpanKindInternal    badgerSpanKind = 0x87
	badgerSpanKindConsumer    badgerSpanKind = 0x88
	badgerSpanKindProducer    badgerSpanKind = 0x89
	badgerSpanKindServer      badgerSpanKind = 0x90
	badgerSpanKindClient      badgerSpanKind = 0x91
)

var toSpanKind = map[badgerSpanKind]model.SpanKind{
	badgerSpanKindInternal:    model.SpanKindInternal,
	badgerSpanKindConsumer:    model.SpanKindConsumer,
	badgerSpanKindProducer:    model.SpanKindProducer,
	badgerSpanKindServer:      model.SpanKindServer,
	badgerSpanKindClient:      model.SpanKindClient,
	badgerSpanKindUnspecified: model.SpanKindUnspecified,
}

var fromSpanKind = map[model.SpanKind]badgerSpanKind{
	model.SpanKindInternal:    badgerSpanKindInternal,
	model.SpanKindConsumer:    badgerSpanKindConsumer,
	model.SpanKindProducer:    badgerSpanKindProducer,
	model.SpanKindServer:      badgerSpanKindServer,
	model.SpanKindClient:      badgerSpanKindClient,
	model.SpanKindUnspecified: badgerSpanKindUnspecified,
}

func (b badgerSpanKind) spanKind() model.SpanKind {
	if kind, ok := toSpanKind[b]; ok {
		return kind
	}
	return model.SpanKindUnspecified
}

func (b badgerSpanKind) validateAndGet() badgerSpanKind {
	switch b {
	case badgerSpanKindInternal, badgerSpanKindConsumer, badgerSpanKindProducer, badgerSpanKindServer, badgerSpanKindClient:
		return b
	default:
		return badgerSpanKindUnspecified
	}
}

func getBadgerSpanKind(kind model.SpanKind) badgerSpanKind {
	if bKind, ok := fromSpanKind[kind]; ok {
		return bKind
	}
	return badgerSpanKindUnspecified
}

func getBadgerSpanKindFromByte(kind byte) badgerSpanKind {
	badgerKind := badgerSpanKind(kind)
	return badgerKind.validateAndGet()
}

// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"strconv"

	"github.com/jaegertracing/jaeger/model"
)

type badgerSpanKind int

const (
	badgerSpanKindUnspecified badgerSpanKind = iota
	badgerSpanKindInternal
	badgerSpanKindConsumer
	badgerSpanKindProducer
	badgerSpanKindServer
	badgerSpanKindClient
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

func (b badgerSpanKind) String() string {
	return strconv.Itoa(int(b))
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

func getBadgerKindFromString(kind string) badgerSpanKind {
	iKind, err := strconv.Atoi(kind)
	if err != nil {
		return badgerSpanKindUnspecified
	}
	bKind := badgerSpanKind(iKind)
	return bKind.validateAndGet()
}

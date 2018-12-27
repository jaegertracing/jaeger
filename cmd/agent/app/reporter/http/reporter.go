package http

import (
	"bytes"
	"errors"

	"go.uber.org/zap"

	thrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/http/client"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Reporter reports data to collector via HTTP.
type Reporter struct {
	client            client.Client
	collectorEndpoint string
	logger            *zap.Logger
}

type jaegerBatch jaeger.Batch

// New creates HTTP reporter.
func New(client client.Client, collectorEndpoint string, logger *zap.Logger) *Reporter {
	return &Reporter{
		client:            client,
		collectorEndpoint: collectorEndpoint,
		logger:            logger,
	}
}

// EmitBatch implements EmitBatch of Reporter
func (r *Reporter) EmitBatch(b *jaeger.Batch) error {
	body, err := serializeJaeger(b)
	if err != nil {
		return err
	}
	err = r.client.Post(body)
	if err != nil {
		return err
	}
	return nil
}

// EmitZipkinBatch are NOT implmented yet
func (r *Reporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return errors.New("Not Implemented Method EmitZipkinBatch")
}

func serializeJaeger(b *jaeger.Batch) (*bytes.Buffer, error) {
	buf := thrift.NewTMemoryBuffer()
	t := thrift.NewTBinaryProtocolTransport(buf)
	if err := b.Write(t); err != nil {
		return nil, err
	}
	return buf.Buffer, nil
}

// CollectorEndpoint returns the collectorEndpoint of reporter
func (r *Reporter) CollectorEndpoint() string {
	return r.collectorEndpoint
}

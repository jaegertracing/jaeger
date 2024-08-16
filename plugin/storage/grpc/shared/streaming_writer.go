// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var _ spanstore.Writer = (*streamingSpanWriter)(nil)

const (
	defaultMaxPoolSize = 1000
)

// streamingSpanWriter wraps storage_v1.StreamingSpanWriterPluginClient into spanstore.Writer
type streamingSpanWriter struct {
	client     storage_v1.StreamingSpanWriterPluginClient
	streamPool chan storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamClient
	closed     atomic.Bool
}

func newStreamingSpanWriter(client storage_v1.StreamingSpanWriterPluginClient) *streamingSpanWriter {
	s := &streamingSpanWriter{
		client:     client,
		streamPool: make(chan storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamClient, defaultMaxPoolSize),
	}
	return s
}

// WriteSpan write span into stream
func (s *streamingSpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	stream, err := s.getStream(ctx)
	if err != nil {
		return fmt.Errorf("plugin getStream error: %w", err)
	}
	if err := stream.Send(&storage_v1.WriteSpanRequest{Span: span}); err != nil {
		return fmt.Errorf("plugin Send error: %w", err)
	}
	s.putStream(stream)
	return nil
}

func (s *streamingSpanWriter) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return errors.New("already closed")
	}
	close(s.streamPool)
	for stream := range s.streamPool {
		if _, err := stream.CloseAndRecv(); err != nil {
			return err
		}
	}
	return nil
}

func (s *streamingSpanWriter) getStream(ctx context.Context) (storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamClient, error) {
	select {
	case st, ok := <-s.streamPool:
		if ok {
			return st, nil
		}
		return nil, fmt.Errorf("plugin is closed")
	default:
		return s.client.WriteSpanStream(ctx)
	}
}

func (s *streamingSpanWriter) putStream(stream storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamClient) error {
	if s.closed.Load() {
		_, err := stream.CloseAndRecv()
		return err
	}
	select {
	case s.streamPool <- stream:
		return nil
	default:
		_, err := stream.CloseAndRecv()
		return err
	}
}

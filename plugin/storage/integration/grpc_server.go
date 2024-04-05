// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"net"
	"sync"

	googleGRPC "google.golang.org/grpc"

	grpcMemory "github.com/jaegertracing/jaeger/plugin/storage/grpc/memory"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCServer struct {
	errChan chan error
	server  *googleGRPC.Server
	wg      sync.WaitGroup
}

func NewGRPCServer() (*GRPCServer, error) {
	return &GRPCServer{errChan: make(chan error, 1)}, nil
}

func (s *GRPCServer) Start() error {
	if s.server != nil {
		if err := s.Close(); err != nil {
			return err
		}
	}

	memStorePlugin := grpcMemory.NewStoragePlugin(memory.NewStore(), memory.NewStore())

	s.server = googleGRPC.NewServer()
	queryPlugin := shared.StorageGRPCPlugin{
		Impl:        memStorePlugin,
		ArchiveImpl: memStorePlugin,
	}

	if err := queryPlugin.RegisterHandlers(s.server); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", ports.PortToHostPort(ports.RemoteStorageGRPC))
	if err != nil {
		return err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err = s.server.Serve(listener); err != nil {
			select {
			case s.errChan <- err:
			default:
			}
		}
	}()
	return nil
}

func (s *GRPCServer) Close() error {
	if s.server == nil {
		return nil
	}

	s.server.GracefulStop()
	s.server = nil
	s.wg.Wait()
	select {
	case err := <-s.errChan:
		return err
	default:
	}
	return nil
}

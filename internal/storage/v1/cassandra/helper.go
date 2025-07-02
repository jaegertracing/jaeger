// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
)

type mockSessionBuilder struct {
	index    int
	sessions []*mocks.Session
	errors   []error
}

func (m *mockSessionBuilder) add(session *mocks.Session, err error) *mockSessionBuilder {
	m.sessions = append(m.sessions, session)
	m.errors = append(m.errors, err)
	return m
}

func (m *mockSessionBuilder) build(*config.Configuration) (cassandra.Session, error) {
	session := m.sessions[m.index]
	err := m.errors[m.index]
	m.index++
	return session, err
}

func MockSession(f *Factory, session *mocks.Session, err error) {
	f.sessionBuilderFn = new(mockSessionBuilder).add(session, err).build
}

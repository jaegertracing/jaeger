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

package flags

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"
)

const (
	// CassandraStorageType is the storage type flag denoting a Cassandra backing store
	CassandraStorageType = "cassandra"
	// MemoryStorageType is the storage type flag denoting an in-memory store
	MemoryStorageType = "memory"
	// ESStorageType is the storage type flag denoting an ElasticSearch backing store
	ESStorageType = "elasticsearch"
)

// ErrUnsupportedStorageType is the error when dealing with an unsupported storage type
var ErrUnsupportedStorageType = errors.New("Storage Type is not supported")

// SpanStorage defines common settings for Span Storage.
var SpanStorage = spanStorage{}

// DependencyStorage defines common settings for Dependency Storage.
var DependencyStorage = dependencyStorage{}

type logging struct {
	Level string
}

type spanStorage struct {
	Type string
}

type dependencyStorage struct {
	Type          string
	DataFrequency time.Duration
}

type cassandraOptions struct {
	ConnectionsPerHost int
	MaxRetryAttempts   int
	QueryTimeout       time.Duration
	Servers            string
	Port               int
	Keyspace           string
}

func (co cassandraOptions) ServerList() []string {
	return strings.Split(co.Servers, ",")
}

func init() {
	flag.StringVar(&SpanStorage.Type, "span-storage.type", CassandraStorageType, fmt.Sprintf("The type of span storage backend to use, options are currently [%v,%v]", CassandraStorageType, MemoryStorageType))

	flag.StringVar(&DependencyStorage.Type, "dependency-storage.type", CassandraStorageType, fmt.Sprintf("The type of dependency storage backend to use, options are currently [%v,%v]", CassandraStorageType, MemoryStorageType))
	flag.DurationVar(&DependencyStorage.DataFrequency, "dependency-storage.data-frequency", time.Hour*24, "Frequency of service dependency calculations")
}

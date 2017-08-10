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

package builder

import (
	"time"

	"github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/plugin/storage/cassandra/dependencystore"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore"
)

func (sb *StorageBuilder) newCassandraBuilder(sessionBuilder config.SessionBuilder, dependencyDataFreq time.Duration) error {
	session, err := sessionBuilder.NewSession()
	if err != nil {
		return err
	}

	sb.SpanReader = spanstore.NewSpanReader(session, sb.metricsFactory, sb.logger)
	sb.DependencyReader = dependencystore.NewDependencyStore(session, dependencyDataFreq, sb.metricsFactory, sb.logger)
	return nil
}

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

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
	"github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/plugin/storage/es/dependencystore"
	"github.com/uber/jaeger/plugin/storage/es/spanstore"
)

func (sb *StorageBuilder) newESBuilder(builder config.ClientBuilder) error {
	client, err := builder.NewClient()
	if err != nil {
		return err
	}

	sb.SpanReader = spanstore.NewSpanReader(client, sb.logger, builder.GetMaxSpanAge(), sb.metricsFactory)
	sb.DependencyReader = dependencystore.NewDependencyStore(client, sb.logger)
	return nil
}

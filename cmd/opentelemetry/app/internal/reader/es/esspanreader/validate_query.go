// Copyright (c) 2020 The Jaeger Authors.
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

package esspanreader

import (
	"errors"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	errNilQuery                   = errors.New("query is nil")
	errServiceNameNotSet          = errors.New("service name must be set")
	errStartAndEndTimeNotSet      = errors.New("start and end time must be set")
	errStartTimeMinGreaterThanMax = errors.New("start time minimum is above maximum")
	errDurationMinGreaterThanMax  = errors.New("duration minimum is above maximum")
)

func validateQuery(p *spanstore.TraceQueryParameters) error {
	if p == nil {
		return errNilQuery
	}
	if p.ServiceName == "" && len(p.Tags) > 0 {
		return errServiceNameNotSet
	}
	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return errStartAndEndTimeNotSet
	}
	if p.StartTimeMax.Before(p.StartTimeMin) {
		return errStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return errDurationMinGreaterThanMax
	}
	return nil
}

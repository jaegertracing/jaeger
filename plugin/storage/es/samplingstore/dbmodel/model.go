// Copyright (c) 2024 The Jaeger Authors.
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

package dbmodel

import (
	"time"
)

type Throughput struct {
	Service       string
	Operation     string
	Count         int64
	Probabilities map[string]struct{}
}

type TimeThroughput struct {
	Timestamp  time.Time  `json:"timestamp"`
	Throughput Throughput `json:"throughputs"`
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities map[string]map[string]float64
	QPS           map[string]map[string]float64
}

type TimeProbabilitiesAndQPS struct {
	Timestamp           time.Time           `json:"timestamp"`
	ProbabilitiesAndQPS ProbabilitiesAndQPS `json:"probabilitiesandqps"`
}

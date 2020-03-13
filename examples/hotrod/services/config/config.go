// Copyright (c) 2019 The Jaeger Authors.
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

package config

import (
	"time"
)

var (
	// 'frontend' service

	// RouteWorkerPoolSize is the size of the worker pool used to query `route` service.
	// Can be overwritten from command line.
	RouteWorkerPoolSize = 3

	// 'customer' service

	// MySQLGetDelay is how long retrieving a customer record takes.
	// Using large value mostly because I cannot click the button fast enough to cause a queue.
	MySQLGetDelay = 300 * time.Millisecond

	// MySQLGetDelayStdDev is standard deviation
	MySQLGetDelayStdDev = MySQLGetDelay / 10

	// MySQLMutexDisabled controls whether there is a mutex guarding db query execution.
	// When not disabled it simulates a misconfigured connection pool of size 1.
	MySQLMutexDisabled = false

	// 'driver' service

	// RedisFindDelay is how long finding closest drivers takes.
	RedisFindDelay = 20 * time.Millisecond

	// RedisFindDelayStdDev is standard deviation.
	RedisFindDelayStdDev = RedisFindDelay / 4

	// RedisGetDelay is how long retrieving a driver record takes.
	RedisGetDelay = 10 * time.Millisecond

	// RedisGetDelayStdDev is standard deviation
	RedisGetDelayStdDev = RedisGetDelay / 4

	// 'route' service

	// RouteCalcDelay is how long a route calculation takes
	RouteCalcDelay = 50 * time.Millisecond

	// RouteCalcDelayStdDev is standard deviation
	RouteCalcDelayStdDev = RouteCalcDelay / 4
)

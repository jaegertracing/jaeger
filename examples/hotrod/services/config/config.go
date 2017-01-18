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

package config

import (
	"time"
)

var (
	// 'frontend' service

	// WorkerPoolSize is the size of goroutine pool used to query for routes
	WorkerPoolSize = 3

	// 'customer' service

	// MySQLGetDelay is how long retrieving a customer record takes
	MySQLGetDelay = 200 * time.Millisecond

	// MySQLGetDelayStdDev is standard deviation
	MySQLGetDelayStdDev = MySQLGetDelay / 4

	// 'driver' service

	// RedisFindDelay is how long finding closest drivers takes
	RedisFindDelay = 20 * time.Millisecond

	// RedisFindDelayStdDev is standard deviation
	RedisFindDelayStdDev = RedisFindDelay / 4

	// RedisGetDelay is how long retrieving a driver record takes
	RedisGetDelay = 10 * time.Millisecond

	// RedisGetDelayStdDev is standard deviation
	RedisGetDelayStdDev = RedisGetDelay / 4

	// 'route' service

	// RouteCalcDelay is how long a route calculation takes
	RouteCalcDelay = 50 * time.Millisecond

	// RouteCalcDelayStdDev is standard deviation
	RouteCalcDelayStdDev = RouteCalcDelay / 4
)

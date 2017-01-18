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

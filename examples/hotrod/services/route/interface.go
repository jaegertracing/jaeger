package route

import "context"
import "time"

// Route describes a route between Pickup and Dropoff locations and expected time to arrival.
type Route struct {
	Pickup  string
	Dropoff string
	ETA     time.Duration
}

// Interface exposed by the Driver service.
type Interface interface {
	FindRoute(ctx context.Context, pickup, dropoff string) (*Route, error)
}

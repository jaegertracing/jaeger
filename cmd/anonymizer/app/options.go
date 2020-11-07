package app

import "github.com/jaegertracing/jaeger/ports"

const (
	DefaultQueryGRPCHost    = "localhost"
	DefaultQueryGRPCPort    = ports.QueryHTTP
	DefaultOutputDir        = "/tmp"
	DefaultHashStandardTags = true
	DefaultHashCustomTags   = false
	DefaultHashLogs         = false
	DefaultHashProcess      = false
	DefaultMaxSpansCount    = -1
)

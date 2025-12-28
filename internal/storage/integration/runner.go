package integration

import "time"

type Options struct {
	GRPCEndpoint string
	Timeout      time.Duration
}

func RunAll(opts Options) error {
	return nil
}

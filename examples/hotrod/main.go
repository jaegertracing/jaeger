// Copyright (c) 2017 Uber Technologies, Inc.

package main

import (
	"runtime"

	"github.com/uber/jaeger/examples/hotrod/cmd"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	cmd.Execute()
}

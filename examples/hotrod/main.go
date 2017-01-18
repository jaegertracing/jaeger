// Copyright © 2017 Uber Technologies, Inc.

package main

import (
	"runtime"

	"code.uber.internal/infra/jaeger-demo/cmd"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	cmd.Execute()
}

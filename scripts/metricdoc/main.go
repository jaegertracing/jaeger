// Copyright (c) 2018 The Jaeger Authors.
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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jaegertracing/jaeger/scripts/metricdoc/app"
)

func printUsage() {
	fmt.Println("usage: metricdoc PACKAGE...")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		panic("invalid arguments")
	}
	listArgs := []string{"list"}
	listArgs = append(listArgs, os.Args[1:]...)
	cmd := exec.Command("go", listArgs...)
	out, err := cmd.Output()
	if err != nil {
		panic(fmt.Sprintf("failed to resolve paths: %s", err.Error()))
	}
	paths := strings.Split(string(out), "\n")
	app.GenerateDoc(paths, os.Stdout)
}

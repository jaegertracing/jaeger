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
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/structtag"
)

func printUsage() {
	fmt.Println("usage: metricdoc PACKAGE...")
}

type metricDoc struct {
	App         string
	Metric      string
	Tags        string
	Description string
}

func (m metricDoc) WriteRow(w io.Writer) {
	w.Write(
		[]byte(fmt.Sprintln("|", strings.Join([]string{m.App, m.Metric, m.Tags, m.Description}, " | "), "|")))
}

func visitPackage(pkg string, node ast.Node) bool {
	field, ok := node.(*ast.Field)
	if !ok {
		return true
	}

	if field.Tag == nil {
		return true
	}

	tagStr, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		tagStr = field.Tag.Value
	}
	tags, err := structtag.Parse(tagStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing tags: %s: %s\n", err.Error(), field.Tag.Value)
		return false
	}

	metricTag, _ := tags.Get("metric")
	if metricTag != nil {
		m := &metricDoc{
			App:    pkg,
			Metric: metricTag.Value(),
		}
		tagsTag, _ := tags.Get("tags")
		if tagsTag != nil {
			m.Tags = tagsTag.Value()
		}

		if field.Doc != nil {
			m.Description = strings.Trim(field.Doc.Text(), " \n\t")
		}

		m.WriteRow(os.Stdout)
	}
	return true
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
	filter := func(i os.FileInfo) bool {
		return !strings.HasSuffix(i.Name(), "_test.go")
	}
	for _, path := range paths {
		fset := token.NewFileSet()
		path = filepath.Join(build.Default.GOPATH, "src", path)
		pkgs, err := parser.ParseDir(fset, path, filter, parser.ParseComments)
		if err != nil {
			panic(fmt.Sprintf("failed to parse directory: %s", err.Error()))
		}

		for pkgName, pkg := range pkgs {
			ast.Inspect(pkg, func(node ast.Node) bool {
				return visitPackage(pkgName, node)
			})
		}
	}
}

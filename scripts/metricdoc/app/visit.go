package app

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/structtag"
)

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

func GenerateDoc(paths []string, writer io.Writer) {
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
				return visit(pkgName, writer, node)
			})
		}
	}
}

func visit(pkg string, writer io.Writer, node ast.Node) bool {
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

		m.WriteRow(writer)
	}
	return true
}

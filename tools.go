// +build tools

package jaeger

import (
	_ "github.com/mjibson/esc"
	_ "github.com/sectioneight/md-to-godoc"
	_ "github.com/securego/gosec/cmd/gosec"
	_ "github.com/wadey/gocovmerge"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/tools/cmd/cover"
	_ "honnef.co/go/tools/cmd/staticcheck"
)

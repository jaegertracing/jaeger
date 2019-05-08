// +build tools

package jaeger

import (
	_ "github.com/gogo/protobuf/protoc-gen-gogo"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
	_ "github.com/mjibson/esc"
	_ "github.com/sectioneight/md-to-godoc"
	_ "github.com/securego/gosec/cmd/gosec"
	_ "github.com/vektra/mockery/cmd/mockery"
	_ "github.com/wadey/gocovmerge"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/tools/cmd/cover"
	_ "honnef.co/go/tools/cmd/staticcheck"
	// _ "github.com/mwitkow/go-proto-validators/protoc-gen-govalidators"
)

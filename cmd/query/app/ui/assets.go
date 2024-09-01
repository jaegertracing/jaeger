package ui

import (
	"embed"
	"net/http"

	"github.com/jaegertracing/jaeger/pkg/gzipfs"
	"github.com/jaegertracing/jaeger/pkg/httpfs"
)

//go:embed actual/*
var actualAssetsFS embed.FS

//go:embed placeholder/index.html
var placeholderAssetsFS embed.FS

func GetStaticFiles() http.FileSystem {
	if _, err := actualAssetsFS.ReadFile("actual/index.html.gz"); err != nil {
		return httpfs.PrefixedFS("placeholder", http.FS(placeholderAssetsFS))
	}

	return httpfs.PrefixedFS("actual", http.FS(gzipfs.New(actualAssetsFS)))
}

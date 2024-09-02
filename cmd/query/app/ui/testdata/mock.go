package testdata

import "embed"

//go:embed actual/*
var TestFS embed.FS

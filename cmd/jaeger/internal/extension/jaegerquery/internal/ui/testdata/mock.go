// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import "embed"

//go:embed actual/*
var TestFS embed.FS

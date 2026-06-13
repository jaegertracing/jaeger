// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package flags defines command line flags that are shared by several jaeger components.
// They are defined in this shared location so that if several components are wired into
// a single binary (e.g. a local container of complete Jaeger backend) they can all share
// the flags.
package flags

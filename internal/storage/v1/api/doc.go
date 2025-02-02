// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package storage is the collection of different storage interfaces that are shared by two or more components.
//
// If a storage is used by only one component, its interface should be defined in the component package, and implementations under ./plugin/storage/{db_name}/{store_type}/....
package storage

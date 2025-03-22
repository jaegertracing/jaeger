// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import "github.com/jaegertracing/jaeger/internal/storage/es"

// Some of the functions of elastic.BulkIndexRequest violate golint rules,
// e.g. Id() should be ID() and BodyJson() should be BodyJSON().

// Id calls this function to internal service.
func (i IndexServiceWrapper) Id(id string) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Id(id), i.bulkService, i.esVersion)
}

// BodyJson calls this function to internal service.
func (i IndexServiceWrapper) BodyJson(body any) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Doc(body), i.bulkService, i.esVersion)
}

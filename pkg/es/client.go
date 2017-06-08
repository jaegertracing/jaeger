// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package es

import (
	"context"

	"github.com/olivere/elastic"
)

// Client is an abstraction for elastic.Client
type Client interface {
	IndexExists(index string) IndicesExistsService
	CreateIndex(index string) IndicesCreateService
	Index() IndexService
}

// IndicesExistsService is an abstraction for elastic.IndicesExistsService
type IndicesExistsService interface {
	Do(ctx context.Context) (bool, error)
}

// IndicesCreateService is an abstraction for elastic.IndicesCreateService
type IndicesCreateService interface {
	Body(mapping string) IndicesCreateService
	Do(ctx context.Context) (*elastic.IndicesCreateResult, error)
}

// IndexService is an abstraction for elastic.IndexService
type IndexService interface {
	Index(index string) IndexService
	Type(typ string) IndexService
	Id(id string) IndexService
	BodyJson(body interface{}) IndexService
	Do(ctx context.Context) (*elastic.IndexResponse, error)
}

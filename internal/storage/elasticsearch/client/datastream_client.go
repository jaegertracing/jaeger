// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// CreateComponentTemplate creates or updates a component template
// (PUT _component_template/<name>) — the reusable mappings/settings building
// blocks that a composable index template references. Idempotent: re-applying
// the same name overwrites the previous content. The argument order mirrors the
// sibling CreateTemplate (template body first, then name).
func (i IndicesClient) CreateComponentTemplate(ctx context.Context, template, name string) error {
	return i.putComposable(ctx, "_component_template/"+name, template, "component template: "+name)
}

// CreateComposableIndexTemplate creates or updates a composable index template
// (PUT _index_template/<name>). It is deliberately separate from CreateTemplate:
// CreateTemplate PUTs the legacy _template on Elasticsearch 7.x / OpenSearch
// (switching to _index_template only on the ES 8 API), whereas this always targets
// the composable _index_template API that data streams require, on every version.
func (i IndicesClient) CreateComposableIndexTemplate(ctx context.Context, template, name string) error {
	return i.putComposable(ctx, "_index_template/"+name, template, "index template: "+name)
}

// putComposable issues an idempotent PUT for a composable template (component or
// index); description identifies it in error messages, mirroring CreateTemplate.
func (i IndicesClient) putComposable(ctx context.Context, endpoint, body, description string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: endpoint,
		method:   http.MethodPut,
		body:     []byte(body),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create " + description)
			}
		}
		return fmt.Errorf("failed to create %s: %w", description, err)
	}
	return nil
}

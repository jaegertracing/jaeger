// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// CreateComponentTemplate creates or updates a composable component template
// (PUT _component_template/<name>). Component templates are the reusable
// mappings/settings building blocks that a data stream's index template composes.
// The PUT is idempotent: re-applying the same name overwrites the previous content.
func (i IndicesClient) CreateComponentTemplate(ctx context.Context, name, template string) error {
	return i.putComposable(ctx, "_component_template/"+name, template, "component template: "+name)
}

// CreateIndexTemplate creates or updates a composable index template
// (PUT _index_template/<name>). Unlike CreateTemplate, which falls back to the
// legacy _template API on Elasticsearch 7.x and OpenSearch, this always targets
// the composable _index_template API that data streams require.
func (i IndicesClient) CreateIndexTemplate(ctx context.Context, name, template string) error {
	return i.putComposable(ctx, "_index_template/"+name, template, "index template: "+name)
}

// putComposable issues an idempotent PUT for a composable template; description
// identifies it in error messages, mirroring CreateTemplate's error handling.
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

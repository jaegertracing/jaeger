// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"net/http"
)

// CreateComponentTemplate creates or updates a composable component template
// (PUT _component_template/<name>). The operation is idempotent: a PUT with the
// same name overwrites the previous content. Component templates are the reusable
// building blocks (mappings, settings) that a data stream's index template
// composes. See RFC 0004 section 3.2.
func (i IndicesClient) CreateComponentTemplate(name, template string) error {
	return i.putComposable("_component_template/"+name, template, "component template: "+name)
}

// CreateIndexTemplate creates or updates a composable index template
// (PUT _index_template/<name>). Unlike CreateTemplate, this always targets the
// composable API, which data streams require regardless of the server version
// (OpenSearch reports as ES 7.x but still supports composable templates).
func (i IndicesClient) CreateIndexTemplate(name, template string) error {
	return i.putComposable("_index_template/"+name, template, "index template: "+name)
}

func (i IndicesClient) putComposable(endpoint, body, what string) error {
	_, err := i.request(elasticRequest{
		endpoint: endpoint,
		method:   http.MethodPut,
		body:     []byte(body),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode != http.StatusOK {
			return responseError.prefixMessage("failed to create " + what)
		}
		return fmt.Errorf("failed to create %s: %w", what, err)
	}
	return nil
}

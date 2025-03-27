// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"net/http"
)

var _ IndexManagementLifecycleAPI = (*ILMClient)(nil)

// ILMClient is a client used to manipulate Index lifecycle management policies.
type ILMClient struct {
	Client
	MasterTimeoutSeconds int
}

// Exists verify if a ILM policy exists
func (i ILMClient) Exists(name string) (bool, error) {
	_, err := i.request(elasticRequest{
		endpoint: "_ilm/policy/" + name,
		method:   http.MethodGet,
	})

	var respError ResponseError
	if errors.As(err, &respError) {
		if respError.StatusCode == http.StatusNotFound {
			return false, nil
		}
	}

	if err != nil {
		return false, fmt.Errorf("failed to get ILM policy: %s, %w", name, err)
	}
	return true, nil
}

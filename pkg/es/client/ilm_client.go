// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

var _ IndexManagementLifecycleAPI = (*ILMClient)(nil)

// ILMClient is a client used to manipulate Index lifecycle management policies.
type ILMClient struct {
	Client
	MasterTimeoutSeconds int
}

type Policy struct {
	ILMPolicy types.IlmPolicy `json:"policy"`
}

// CreateOrUpdate Add or update a ILMPolicy
func (i ILMClient) CreateOrUpdate(name string, ilmPolicy Policy) error {
	body, err := json.Marshal(ilmPolicy)
	if err != nil {
		return err
	}
	_, err = i.request(elasticRequest{
		endpoint: "_ilm/policy/" + name,
		body:     body,
		method:   http.MethodPut,
	})

	var respError ResponseError
	if errors.As(err, &respError) {
		if respError.StatusCode == http.StatusNotFound {
			return nil
		}
	}

	if err != nil {
		return fmt.Errorf("failed to create/update ILM policy: %s, %w", name, err)
	}
	return nil
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

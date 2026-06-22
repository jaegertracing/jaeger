// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cenkalti/backoff/v5"
	"go.uber.org/zap"
)

const ismPolicyBasePath = "_plugins/_ism/policies/"

var _ IndexManagementLifecycleAPI = (*ISMClient)(nil)

// ISMClient manipulates OpenSearch Index State Management (ISM) policies. ISM is
// OpenSearch's equivalent of Elasticsearch ILM; it has no dedicated client in the
// olivere library, so policies are managed through the direct REST endpoint
// (_plugins/_ism/policies/). See RFC 0004 section 3.8.
type ISMClient struct {
	Client
	Logger *zap.Logger
}

// Exists reports whether an ISM policy with the given name exists.
func (i ISMClient) Exists(name string) (bool, error) {
	operation := func() ([]byte, error) {
		bytes, err := i.request(elasticRequest{
			endpoint: ismPolicyBasePath + name,
			method:   http.MethodGet,
		})
		if err != nil {
			i.Logger.Warn(
				"Retryable error while getting ISM policy",
				zap.String("name", name),
				zap.Error(err),
			)
			return bytes, err
		}
		return bytes, nil
	}

	_, err := backoff.Retry(
		context.TODO(),
		operation,
		backoff.WithMaxTries(maxTries),
		backoff.WithMaxElapsedTime(maxElapsedTime),
	)

	var respError ResponseError
	if errors.As(err, &respError) && respError.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get ISM policy: %s, %w", name, err)
	}
	return true, nil
}

// Create installs an ISM policy (PUT _plugins/_ism/policies/<name>). Callers
// should create only when Exists reports false, so existing (possibly
// user-customized) policies are never overwritten. See RFC 0004 section 3.6.
func (i ISMClient) Create(name, policy string) error {
	_, err := i.request(elasticRequest{
		endpoint: ismPolicyBasePath + name,
		method:   http.MethodPut,
		body:     []byte(policy),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode != http.StatusOK {
			return responseError.prefixMessage("failed to create ISM policy: " + name)
		}
		return fmt.Errorf("failed to create ISM policy: %s, %w", name, err)
	}
	return nil
}

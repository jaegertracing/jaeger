// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v6"
	"go.uber.org/zap"
)

const (
	maxTries       = 3
	maxElapsedTime = 5 * time.Second
)

var _ IndexManagementLifecycleAPI = (*ILMClient)(nil)

// ILMClient is a client used to manipulate Index lifecycle management policies.
// It supports both Elasticsearch ILM and OpenSearch ISM APIs.
type ILMClient struct {
	Client
	MasterTimeoutSeconds int
	Logger               *zap.Logger
	// UseOpenSearchISM switches from _ilm/policy/ to _plugins/_ism/policies/.
	UseOpenSearchISM bool
}

// Exists verify if a ILM/ISM policy exists
func (i ILMClient) Exists(ctx context.Context, name string) (bool, error) {
	endpoint := "_ilm/policy/" + name
	if i.UseOpenSearchISM {
		endpoint = "_plugins/_ism/policies/" + name
	}
	operation := func() ([]byte, error) {
		bytes, err := i.request(ctx, elasticRequest{
			endpoint: endpoint,
			method:   http.MethodGet,
		})
		if err != nil {
			i.Logger.Warn(
				"Retryable error while getting ILM/ISM policy",
				zap.String("name", name),
				zap.Error(err),
			)
			return bytes, err
		}
		return bytes, nil
	}

	_, err := backoff.Retry(
		ctx,
		operation,
		backoff.WithMaxTries(maxTries),
		backoff.WithMaxElapsedTime(maxElapsedTime),
	)

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

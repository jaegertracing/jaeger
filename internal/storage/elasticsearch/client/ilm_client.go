// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"
)

const (
	maxRetries      = 3
	initialInterval = 500 * time.Millisecond
	backoffFactor   = 2.0
	maxInterval     = 10 * time.Second
)

var _ IndexManagementLifecycleAPI = (*ILMClient)(nil)

// ILMClient is a client used to manipulate Index lifecycle management policies.
type ILMClient struct {
	Client
	MasterTimeoutSeconds int
}

// Exists verify if a ILM policy exists
func (i ILMClient) Exists(name string) (bool, error) {
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		_, err = i.request(elasticRequest{
			endpoint: "_ilm/policy/" + name,
			method:   http.MethodGet,
		})
		if err == nil {
			return true, nil
		}
		log.Printf("Attempt %d to get ILM policy for %s, failed: %v", attempt+1, name, err)

		backoff := time.Duration(float64(initialInterval) * math.Pow(backoffFactor, float64(attempt)))
		if backoff > maxInterval {
			backoff = maxInterval
		}
		time.Sleep(backoff)
	}

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

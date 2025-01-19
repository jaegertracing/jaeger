// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"net/http"
)

var _ DataStreamAPI = (*DataStreamClient)(nil)

type DataStreamClient struct {
	Client
}

func (ds DataStreamClient) Create(dataStream string) error {
	_, err := ds.request(elasticRequest{
		endpoint: "_data_stream/" + dataStream,
		method:   http.MethodPut,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create data stream: " + dataStream)
			}
		}
		return fmt.Errorf("failed to create data stream: %w", err)
	}
	return nil
}

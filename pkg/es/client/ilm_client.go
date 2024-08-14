// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		endpoint: fmt.Sprintf("_ilm/policy/%s", name),
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

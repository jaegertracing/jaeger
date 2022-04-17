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

package bearertoken

import (
	"errors"
	"net/http"
)

// RoundTripper wraps another http.RoundTripper and injects
// an authentication header with bearer token into requests.
type RoundTripper struct {
	// Transport is the underlying http.RoundTripper being wrapped. Required.
	Transport http.RoundTripper

	// StaticToken is the pre-configured bearer token. Optional.
	StaticToken string

	// OverrideFromCtx enables reading bearer token from Context.
	OverrideFromCtx bool
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.Transport == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}
	token := tr.StaticToken
	if tr.OverrideFromCtx {
		headerToken, _ := GetBearerToken(r.Context())
		if headerToken != "" {
			token = headerToken
		}
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return tr.Transport.RoundTrip(r)
}

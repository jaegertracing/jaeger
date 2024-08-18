// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
)

func additionalHeadersHandler(h http.Handler, additionalHeaders http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		for key, values := range additionalHeaders {
			header[key] = values
		}

		h.ServeHTTP(w, r)
	})
}

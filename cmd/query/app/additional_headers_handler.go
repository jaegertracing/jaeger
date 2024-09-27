// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"

	"go.opentelemetry.io/collector/config/configopaque"
)

func additionalHeadersHandler(h http.Handler, additionalHeaders map[string]configopaque.String) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		for key, value := range additionalHeaders {
			header[key] = []string{value.String()}
		}

		h.ServeHTTP(w, r)
	})
}

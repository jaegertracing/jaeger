// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
)

// PropagationHandler returns a http.Handler containing the logic to extract
// the tenancy header of the http.Request and insert the tenant into request.Context
// for propagation. The token can be accessed via tenancy.GetTenant().
func ExtractTenantHTTPHandler(tc *Manager, h http.Handler) http.Handler {
	if !tc.Enabled {
		return h
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenants := r.Header.Values(tc.Header)
		if len(tenants) > 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("extra tenant header"))
			return
		}

		tenant := ""
		if len(tenants) == 1 {
			tenant = tenants[0]
		}
		if tenant == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("missing tenant header"))
			return
		}

		if !tc.Valid(tenant) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unknown tenant"))
			return
		}

		ctx := WithTenant(r.Context(), tenant)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// MetadataAnnotator returns a function suitable for propagating tenancy
// via github.com/grpc-ecosystem/grpc-gateway/runtime.NewServeMux
func (tc *Manager) MetadataAnnotator() func(context.Context, *http.Request) metadata.MD {
	return func(_ context.Context, req *http.Request) metadata.MD {
		tenants := req.Header.Values(tc.Header)
		if len(tenants) != 1 {
			// The HTTP request lacked the tenancy header, or carried it
			// more than once.  Pass along empty metadata -- the gRPC
			// query service will reject later, as it does for requests
			// with extra tenant headers.
			return metadata.Pairs()
		}
		return metadata.New(map[string]string{
			tc.Header: tenants[0],
		})
	}
}

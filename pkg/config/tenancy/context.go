// Copyright (c) 2022 The Jaeger Authors.
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

package tenancy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jaegertracing/jaeger/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func tenantFromMetadata(md metadata.MD, tenancyHeader string) (string, error) {
	tenants := md.Get(tenancyHeader)
	if len(tenants) < 1 {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	} else if len(tenants) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	return tenants[0], nil
}

// @@@ TODO WRITE TESTS
func (tc *TenancyConfig) GetValidTenantContext(ctx context.Context) (context.Context, error) {
	tenant := storage.GetTenant(ctx)
	// Is the tenant directly in the context?
	if tenant != "" {
		fmt.Printf("@@@ ecs GetValidTenantContext found %q directly in context\n", tenant)
		if !tc.Valid(tenant) {
			fmt.Printf("@@@ ecs GetValidTenantContext: tenant %q not valid\n", tenant)
			return ctx, status.Errorf(codes.PermissionDenied, "missing tenant header")
		}
		return ctx, nil
	}

	// The tenant might be in the metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	var err error
	tenant, err = tenantFromMetadata(md, tc.Header)
	if err != nil {
		return ctx, err
	}
	if !tc.Valid(tenant) {
		return ctx, status.Errorf(codes.PermissionDenied, "unknown tenant")
	}

	// Apply the tenant directly the context (in addition to metadata)
	return storage.WithTenant(ctx, tenant), nil
}

// PropagationHandler returns a http.Handler containing the logic to extract
// the tenancy header of the http.Request and insert the tenant into request.Context
// for propagation. The token can be accessed via storage.GetTenant().
func (tc *TenancyConfig) PropagationHandler(logger *zap.Logger, h http.Handler) http.Handler {
	if !tc.Enabled {
		return h
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get(tc.Header)
		if tenant == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("missing tenant header"))
			return
		}

		if !tc.Valid(tenant) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unknown tenant"))
			return
		}

		h.ServeHTTP(w, r.WithContext(storage.WithTenant(r.Context(), tenant)))
	})
}

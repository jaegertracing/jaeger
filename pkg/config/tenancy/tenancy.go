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
	"runtime/debug"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// TenancyConfig holds the settings for multi-tenant Jaeger
type TenancyConfig struct {
	Enabled bool
	Header  string
	guard   guard
}

// Guard verifies a valid tenant when tenancy is enabled
type guard interface {
	Valid(candidate string) bool
}

type guardedSpanstoreReader struct {
	tenancyHeader string
	reader        spanstore.Reader
	guard         guard
}

type guardedDependencystoreReader struct {
	tenancyHeader string
	reader        dependencystore.Reader
	guard         guard
}

// Options describes the configuration properties for multitenancy
type Options struct {
	Enabled bool
	Header  string
	Tenants []string
}

// NewTenancyConfig creates a tenancy configuration for tenancy Options
func NewTenancyConfig(options *Options) *TenancyConfig {
	return &TenancyConfig{
		Enabled: options.Enabled,
		Header:  options.Header,
		guard:   tenancyGuardFactory(options),
	}
}

func (tc *TenancyConfig) Valid(tenant string) bool {
	return tc.guard.Valid(tenant)
}

// Anything will pass: no header, empty tenant, etc
type tenantDontCare bool

func (tenantDontCare) Valid(candidate string) bool {
	return true
}

// Header is required, tenant must have any non-empty value
type tenantAny bool

func (tenantAny) Valid(candidate string) bool {
	return candidate != ""
}

// Header is required, tenant must be predefined
type tenantList struct {
	tenants map[string]bool
}

func (tl *tenantList) Valid(candidate string) bool {
	_, ok := tl.tenants[candidate]
	return ok
}

func newTenantList(tenants []string) *tenantList {
	tenantMap := make(map[string]bool)
	for _, tenant := range tenants {
		tenantMap[tenant] = true
	}

	return &tenantList{
		tenants: tenantMap,
	}
}

func tenancyGuardFactory(options *Options) guard {
	// Three cases
	// - no tenancy
	// - tenancy, but no guarding by tenant
	// - tenancy, with guarding by a list

	if !options.Enabled {
		return tenantDontCare(true)
	}

	if len(options.Tenants) == 0 {
		return tenantAny(true)
	}

	return newTenantList(options.Tenants)
}

func NewGuardedSpanReader(reader spanstore.Reader, options *Options) spanstore.Reader {
	return &guardedSpanstoreReader{
		tenancyHeader: options.Header,
		guard:         tenancyGuardFactory(options),
		reader:        reader,
	}
}

// @@@ ecs TODO refactor validateTenant
func TenantFromMetadata(md metadata.MD, tenancyHeader string) (string, error) {
	tenants := md.Get(tenancyHeader)
	if len(tenants) < 1 {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	} else if len(tenants) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	return tenants[0], nil
}

func ensureTenant(ctx context.Context, tenancyHeader string) (string, context.Context, error) {
	tenant := storage.GetTenant(ctx)
	// The tenant might be in either directly in the context or through the metadata
	if tenant == "" {
		fmt.Printf("@@@ ecs ensureTenant found no context tenant, checking metadata\n")
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			fmt.Printf("@@@ ecs ensureTenant NO METADATA\n")
			return "", ctx, status.Errorf(codes.PermissionDenied, "missing tenant header")
		}
		fmt.Printf("@@@ ecs ensureTenant found metadata\n")

		var err error
		tenant, err = TenantFromMetadata(md, tenancyHeader)
		fmt.Printf("@@@ ecs ensureTenant value of header %s is %q\n", tenancyHeader, tenant)
		if err != nil {
			return "", ctx, err
		}

		ctx = storage.WithTenant(ctx, tenant)
	}

	return tenant, ctx, nil
}

func (gsr *guardedSpanstoreReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		return nil, err
	}

	return gsr.reader.GetTrace(tenantedCtx, traceID)
}

func (gsr *guardedSpanstoreReader) GetServices(ctx context.Context) ([]string, error) {
	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		return nil, err
	}

	return gsr.reader.GetServices(tenantedCtx)
}

func (gsr *guardedSpanstoreReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		return nil, err
	}

	return gsr.reader.GetOperations(tenantedCtx, query)
}

func (gsr *guardedSpanstoreReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	fmt.Printf("@@@ ecs REACHED gsr.FindTraces, ctx is %#v, a %T\n", ctx, ctx)
	if _, ok := metadata.FromIncomingContext(ctx); ok {
		fmt.Printf("@@@ ecs REACHED gsr.FindTraces, ctx has incoming metadata\n")
	}
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		fmt.Printf("@@@ ecs REACHED gsr.FindTraces, ctx has outgoing metadata\n")
	}
	debug.PrintStack()

	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		fmt.Printf("@@@ ecs REACHED gsr.FindTraces(): tenant %q is NOT VALID\n", tenant)
		return nil, err
	}

	fmt.Printf("@@@ ecs REACHED gsr.FindTraces(): tenant %q is VALID\n", tenant)
	return gsr.reader.FindTraces(tenantedCtx, query)

}

func (gsr *guardedSpanstoreReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		return nil, err
	}

	return gsr.reader.FindTraceIDs(tenantedCtx, query)

}

func (gsr *guardedDependencystoreReader) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	tenant, tenantedCtx, err := ensureTenant(ctx, gsr.tenancyHeader)
	if !gsr.guard.Valid(tenant) {
		return nil, err
	}

	return gsr.reader.GetDependencies(tenantedCtx, endTs, lookback)
}

func NewGuardedDependencyReader(reader dependencystore.Reader, options *Options) dependencystore.Reader {
	return &guardedDependencystoreReader{
		tenancyHeader: options.Header,
		guard:         tenancyGuardFactory(options),
		reader:        reader,
	}
}

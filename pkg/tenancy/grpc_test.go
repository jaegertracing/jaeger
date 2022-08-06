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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTenancyInterceptors(t *testing.T) {
	tests := []struct {
		name       string
		tenancyMgr *Manager
		ctx        context.Context
		errMsg     string
	}{
		{
			name:       "missing tenant context",
			tenancyMgr: NewManager(&Options{Enabled: true}),
			ctx:        context.Background(),
			errMsg:     "rpc error: code = PermissionDenied desc = missing tenant header",
		},
		{
			name:       "invalid tenant context",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"megacorp"}}),
			ctx:        WithTenant(context.Background(), "acme"),
			errMsg:     "rpc error: code = PermissionDenied desc = unknown tenant",
		},
		{
			name:       "valid tenant context",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"acme"}}),
			ctx:        WithTenant(context.Background(), "acme"),
			errMsg:     "",
		},
		{
			name:       "invalid tenant header",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"megacorp"}}),
			ctx:        metadata.NewIncomingContext(context.Background(), map[string][]string{"x-tenant": {"acme"}}),
			errMsg:     "rpc error: code = PermissionDenied desc = unknown tenant",
		},
		{
			name:       "missing tenant header",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"megacorp"}}),
			ctx:        metadata.NewIncomingContext(context.Background(), map[string][]string{}),
			errMsg:     "rpc error: code = PermissionDenied desc = missing tenant header",
		},
		{
			name:       "valid tenant header",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"acme"}}),
			ctx:        metadata.NewIncomingContext(context.Background(), map[string][]string{"x-tenant": {"acme"}}),
			errMsg:     "",
		},
		{
			name:       "extra tenant header",
			tenancyMgr: NewManager(&Options{Enabled: true, Tenants: []string{"acme"}}),
			ctx:        metadata.NewIncomingContext(context.Background(), map[string][]string{"x-tenant": {"acme", "megacorp"}}),
			errMsg:     "rpc error: code = PermissionDenied desc = extra tenant header",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			interceptor := NewGuardingStreamInterceptor(test.tenancyMgr)
			ss := tenantedServerStream{
				context: test.ctx,
			}
			ssi := grpc.StreamServerInfo{}
			handler := func(interface{}, grpc.ServerStream) error {
				// do nothing
				return nil
			}
			err := interceptor(0, &ss, &ssi, handler)
			if test.errMsg == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, test.errMsg, err.Error())
			}

			uinterceptor := NewGuardingUnaryInterceptor(test.tenancyMgr)
			usi := &grpc.UnaryServerInfo{}
			iface := 0
			uhandler := func(ctx context.Context, req interface{}) (interface{}, error) {
				// do nothing
				return req, nil
			}
			_, err = uinterceptor(test.ctx, iface, usi, uhandler)
			if test.errMsg == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, test.errMsg, err.Error())
			}
		})
	}
}

func TestClientUnaryInterceptor(t *testing.T) {
	tm := NewManager(&Options{Enabled: true, Tenants: []string{"acme"}})
	interceptor := NewClientUnaryInterceptor(tm)
	var tenant string
	fakeErr := errors.New("foo")
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		assert.True(t, ok)
		ten, err := tenantFromMetadata(md, tm.Header)
		require.NoError(t, err)
		tenant = ten
		return fakeErr
	}
	ctx := WithTenant(context.Background(), "acme")
	err := interceptor(ctx, "method", "request", "response", nil, invoker)
	assert.Equal(t, "acme", tenant)
	assert.Same(t, fakeErr, err)
}

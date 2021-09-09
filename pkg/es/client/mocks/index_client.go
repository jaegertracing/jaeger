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

package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

type MockIndexAPI struct {
	mock.Mock
}

func (c *MockIndexAPI) GetJaegerIndices(prefix string) ([]client.Index, error) {
	ret := c.Called(prefix)
	return ret.Get(0).([]client.Index), ret.Error(1)
}
func (c *MockIndexAPI) DeleteIndices(indices []client.Index) error {
	ret := c.Called(indices)
	return ret.Error(0)
}
func (c *MockIndexAPI) CreateIndex(index string) error {
	ret := c.Called(index)
	return ret.Error(0)
}
func (c *MockIndexAPI) CreateAlias(aliases []client.Alias) error {
	ret := c.Called(aliases)
	return ret.Error(0)
}
func (c *MockIndexAPI) DeleteAlias(aliases []client.Alias) error {
	ret := c.Called(aliases)
	return ret.Error(0)
}
func (c *MockIndexAPI) CreateTemplate(template, name string) error {
	ret := c.Called(template, name)
	return ret.Error(0)
}
func (c *MockIndexAPI) Rollover(rolloverTarget string, conditions map[string]interface{}) error {
	ret := c.Called(rolloverTarget, conditions)
	return ret.Error(0)
}

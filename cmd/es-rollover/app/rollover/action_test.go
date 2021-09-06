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

package rollover

import (
	"errors"
	"testing"

	"github.com/crossdock/crossdock-go/assert"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/client/mocks"
)

func TestRolloverAction(t *testing.T) {
	readIndices := []client.Index{
		{
			Index: "jaeger-read-span",
			Aliases: map[string]bool{
				"jaeger-span-archive-write": true,
			},
		},
	}

	aliasToCreate := []client.Alias{{Index: "jaeger-read-span", Name: "jaeger-span-archive-read", IsWriteIndex: false}}
	tests := []struct {
		name                 string
		conditions           string
		unmarshalErrExpected bool
		getJaegerIndicesErr  error
		rolloverErr          error
		createAliasErr       error
		expectedError        bool
	}{
		{
			name:          "success",
			conditions:    "{\"max_age\": \"2d\"}",
			expectedError: false,
		},
		{
			name:                "get jaeger indices error",
			conditions:          "{\"max_age\": \"2d\"}",
			expectedError:       true,
			getJaegerIndicesErr: errors.New("unable to get indices"),
		},
		{
			name:          "rollover error",
			conditions:    "{\"max_age\": \"2d\"}",
			expectedError: true,
			rolloverErr:   errors.New("unable to rollover"),
		},
		{
			name:           "create alias error",
			conditions:     "{\"max_age\": \"2d\"}",
			expectedError:  true,
			createAliasErr: errors.New("unable to create alias"),
		},
		{
			name:                 "unmarshal conditions error",
			conditions:           "{\"max_age\" \"2d\"},",
			unmarshalErrExpected: true,
			createAliasErr:       errors.New("unable to create alias"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexClient := &mocks.MockIndexAPI{}

			rolloverAction := Action{
				Config: Config{
					Conditions: test.conditions,
					Config: app.Config{
						Archive: true,
					},
				},
				IndicesClient: indexClient,
			}

			if !test.unmarshalErrExpected {
				if test.rolloverErr == nil {
					indexClient.On("GetJaegerIndices", "").Return(readIndices, test.getJaegerIndicesErr)
					if test.getJaegerIndicesErr == nil {
						indexClient.On("CreateAlias", aliasToCreate).Return(test.createAliasErr)
					}
				}
				indexClient.On("Rollover", "jaeger-span-archive-write", map[string]interface{}{"max_age": "2d"}).Return(test.rolloverErr)
			}

			err := rolloverAction.Do()
			if test.expectedError || test.unmarshalErrExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			indexClient.AssertExpectations(t)
		})
	}

}

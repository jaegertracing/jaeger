// Copyright (c) 2020 The Jaeger Authors.
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

package es

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {

	str := "tesing parse"
	writer := new(bytes.Buffer)
	testTemplateBuilder := TextTemplateBuilder{}
	parsedStr, err := testTemplateBuilder.Parse(str)
	require.NoError(t, err)
	err = parsedStr.Execute(writer, "")
	require.NoError(t, err)
	assert.Equal(t, str, writer.String())

}

func TestNewTextTemplateBuilder(t *testing.T){
	textBuilder := NewTextTemplateBuilder()
	assert.IsType(t, TextTemplateBuilder{},textBuilder)
}
// Copyright (c) 2023 The Jaeger Authors.
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

package mappings

import (
	"fmt"
	"strconv"
)

type FieldType int

func ParseFieldType(v any) FieldType {
	if value, ok := v.(bool); ok {
		if value {
			return ObjectFieldType
		}
		return NestedFieldType
	} else if value, ok := v.(string); ok {
		switch value {
		case "object", "true":
			return ObjectFieldType
		default:
			return NestedFieldType
		}
	}
	return NestedFieldType
}

func (field FieldType) Format(f fmt.State, verb rune) {
	str := "object"
	if field == NestedFieldType {
		str = "nested"
	}

	switch verb {
	case 'v':
		_, _ = f.Write([]byte(str))
	default:
		_, _ = f.Write([]byte(strconv.Itoa(int(field))))
	}
}

const (
	NestedFieldType FieldType = iota
	ObjectFieldType
)

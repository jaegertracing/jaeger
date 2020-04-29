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

package defaults

import (
	"reflect"

	"github.com/imdario/mergo"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
)

// MergeConfigs merges two configs.
// The src is merged into dst.
func MergeConfigs(dst, src *configmodels.Config) error {
	if src == nil {
		return nil
	}
	err := mergo.Merge(dst, src,
		mergo.WithOverride,
		mergo.WithAppendSlice,
		mergo.WithTransformers(transformerSliceUniqueValues{}))
	if err != nil {
		return err
	}
	return nil
}

// transformerSliceUniqueValues merges unique values of two string slices.
type transformerSliceUniqueValues struct {
}

func (st transformerSliceUniqueValues) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	if typ == reflect.TypeOf([]string{}) {
		return func(dst, src reflect.Value) error {
			dst.Interface()
			if dst.CanSet() {
				dstVal := dst.Interface().([]string)
				m := map[string]bool{}
				for _, v := range dstVal {
					m[v] = true
				}
				srcVal := src.Interface().([]string)
				for _, v := range srcVal {
					if !m[v] {
						srcVal = append(srcVal, v)
						dst.Set(reflect.Append(dst, reflect.ValueOf(v)))
					}
				}
			}
			return nil
		}
	}
	return nil
}

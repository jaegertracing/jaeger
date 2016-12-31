// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package adjuster

import "github.com/uber/jaeger/model"

// Anonymizer returns an adjuster that modifies the trace to remove any information that might
// be considered private or confidential. Examples are service names, port numbers, operations,
// and any user-added data.
//
// This allows real production traces to safely be kept in public repositories for use in test
// suites, after anonymization.
//
// A custom mapping is provided that dictates how to anonymize data. If a value is encountered that
// that not in the mapping, the mapping is modified to include the new value. Modifications are
// deterministic.
//
// This adjuster never returns any errors.
func Anonymizer(mapping AnonymizerMapping) Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		return trace, nil
	})
}

// AnonymizerMapping dictates how anonymization is done.
//
// It is a map from anonymized field name -> field value -> anonymized value
//
// An example of a mapping:
//var mapping = AnonymizerMapping{
//	adjuster.AnonymizedService: {
//		"user-service":        "service1",
//		"billing-service":     "service2",
//		"password-hash-store": "service3",
//	},
//	adjuster.AnonymizedPort: {
//		"5103": "1001",
//		"7531": "1002",
//	},
//	adjuster.AnonymizedOperation: {
//		"authenticate-user": "operation1",
//	},
//}
type AnonymizerMapping map[AnonymizedField]map[string]string

// AnonymizedField is a field that is supported for anonymization
type AnonymizedField int

const (
	// AnonymizedService anonymizes the service name in processes
	AnonymizedService AnonymizedField = iota + 1
	// AnonymizedPort anonymizes the port number in spans
	AnonymizedPort
	// AnonymizedOperation anonymizes the operation in spans
	AnonymizedOperation
)

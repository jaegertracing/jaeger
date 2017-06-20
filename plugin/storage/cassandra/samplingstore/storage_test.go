// Copyright (c) 2017 Uber Technologies, Inc.
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

package samplingstore

import (
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/samplingstore"
)

var (
	testTime = time.Date(2017, time.January, 24, 11, 15, 17, 12345, time.UTC)
)

type samplingStoreTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	store     *SamplingStore
}

func withSamplingStore(fn func(r *samplingStoreTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metrics.NewLocalFactory(0)
	r := &samplingStoreTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		store:     New(session, metricsFactory, logger),
	}
	fn(r)
}

func TestNewSamplingStore(t *testing.T) {
	withSamplingStore(func(s *samplingStoreTest) {
		var store samplingstore.Store = s.store // check API conformance
		assert.NotNil(t, store)
	})
}

func TestInsertThroughput(t *testing.T) {
	withSamplingStore(func(s *samplingStoreTest) {
		query := &mocks.Query{}
		query.On("Exec").Return(nil)

		var args []interface{}
		captureArgs := mock.MatchedBy(func(v []interface{}) bool {
			args = v
			return true
		})

		s.session.On("Query", mock.AnythingOfType("string"), captureArgs).Return(query)

		throughput := []*model.Throughput{
			{
				Service:   "svc,withcomma",
				Operation: "op,withcomma",
				Count:     40,
			},
		}
		err := s.store.InsertThroughput(throughput)
		assert.NoError(t, err)

		assert.Len(t, args, 3)
		if _, ok := args[0].(int64); !ok {
			assert.Fail(t, "expecting first arg as int64", "received: %+v", args)
		}
		if _, ok := args[1].(gocql.UUID); !ok {
			assert.Fail(t, "expecting second arg as gocql.UUID", "received: %+v", args)
		}
		if d, ok := args[2].(string); ok {
			assert.Equal(t, "\"svc,withcomma\",\"op,withcomma\",40,\n", d)
		} else {
			assert.Fail(t, "expecting third arg as string", "received: %+v", args)
		}
	})
}

func TestInsertProbabilitiesAndQPS(t *testing.T) {
	withSamplingStore(func(s *samplingStoreTest) {
		query := &mocks.Query{}
		query.On("Exec").Return(nil)

		var args []interface{}
		captureArgs := mock.MatchedBy(func(v []interface{}) bool {
			args = v
			return true
		})

		s.session.On("Query", mock.AnythingOfType("string"), captureArgs).Return(query)

		hostname := "hostname"
		probabilities := model.ServiceOperationProbabilities{
			"svc": map[string]float64{
				"op": 0.84,
			},
		}
		qps := model.ServiceOperationQPS{
			"svc": map[string]float64{
				"op": 40,
			},
		}

		err := s.store.InsertProbabilitiesAndQPS(hostname, probabilities, qps)
		assert.NoError(t, err)

		assert.Len(t, args, 4)
		if d, ok := args[0].(int); ok {
			assert.Equal(t, 1, d)
		} else {
			assert.Fail(t, "expecting first arg as int", "received: %+v", args)
		}
		if _, ok := args[1].(gocql.UUID); !ok {
			assert.Fail(t, "expecting second arg as gocql.UUID", "received: %+v", args)
		}
		if d, ok := args[2].(string); ok {
			assert.Equal(t, hostname, d)
		} else {
			assert.Fail(t, "expecting third arg as string", "received: %+v", args)
		}
		if d, ok := args[3].(string); ok {
			assert.Equal(t, "svc,op,0.84,40\n", d)
		} else {
			assert.Fail(t, "expecting fourth arg as string", "received: %+v", args)
		}
	})
}

func TestGetThroughput(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
	}{
		{
			caption: "success",
		},
		{
			caption:       "failure",
			queryError:    errors.New("query error"),
			expectedError: "Error reading throughput from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSamplingStore(func(s *samplingStoreTest) {
				scanMatcher := func() interface{} {
					throughputStr := []string{
						"\"svc,withcomma\",\"op,withcomma\",40,\"0.1,\"\n",
						"svc,op,50,\n",
					}
					scanFunc := func(args []interface{}) bool {
						if len(throughputStr) == 0 {
							return false
						}
						for _, arg := range args {
							if ptr, ok := arg.(*string); ok {
								*ptr = throughputStr[0]
								break
							}
						}
						throughputStr = throughputStr[1:]
						return true
					}
					return mock.MatchedBy(scanFunc)
				}

				iter := &mocks.Iterator{}
				iter.On("Scan", scanMatcher()).Return(true)
				iter.On("Scan", matchEverything()).Return(false)
				iter.On("Close").Return(testCase.queryError)

				query := &mocks.Query{}
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				throughput, err := s.store.GetThroughput(testTime, testTime)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					assert.Len(t, throughput, 2)
					assert.Equal(t,
						model.Throughput{
							Service:       "svc,withcomma",
							Operation:     "op,withcomma",
							Count:         40,
							Probabilities: map[string]struct{}{"0.1": {}}},
						*throughput[0],
					)
					assert.Equal(t,
						model.Throughput{
							Service:       "svc",
							Operation:     "op",
							Count:         50,
							Probabilities: map[string]struct{}{}},
						*throughput[1],
					)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
			})
		})
	}
}

func TestGetProbabilitiesAndQPS(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
	}{
		{
			caption: "success",
		},
		{
			caption:       "failure",
			queryError:    errors.New("query error"),
			expectedError: "Error reading probabilities and qps from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSamplingStore(func(s *samplingStoreTest) {
				scanMatcher := func() interface{} {
					probabilitiesAndQPSStr := []string{
						"svc,op,0.84,40\n",
					}
					scanFunc := func(args []interface{}) bool {
						if len(probabilitiesAndQPSStr) == 0 {
							return false
						}
						for _, arg := range args {
							if ptr, ok := arg.(*string); ok {
								*ptr = probabilitiesAndQPSStr[0]
								break
							}
						}
						probabilitiesAndQPSStr = probabilitiesAndQPSStr[1:]
						return true
					}
					return mock.MatchedBy(scanFunc)
				}

				iter := &mocks.Iterator{}
				iter.On("Scan", scanMatcher()).Return(true)
				iter.On("Scan", scanMatcher()).Return(false)
				iter.On("Close").Return(testCase.queryError)

				query := &mocks.Query{}
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				hostProbabilitiesAndQPS, err := s.store.GetProbabilitiesAndQPS(testTime, testTime)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					assert.Len(t, hostProbabilitiesAndQPS, 1)
					pAndQ := hostProbabilitiesAndQPS[""]
					assert.Len(t, pAndQ, 1)
					assert.Equal(t, 0.84, pAndQ[0]["svc"]["op"].Probability)
					assert.Equal(t, float64(40), pAndQ[0]["svc"]["op"].QPS)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
			})
		})
	}
}

func TestGetLatestProbabilities(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
	}{
		{
			caption: "success",
		},
		{
			caption:       "failure",
			queryError:    errors.New("query error"),
			expectedError: "Error reading probabilities from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSamplingStore(func(s *samplingStoreTest) {
				scanMatcher := func() interface{} {
					probabilitiesStr := []string{
						"svc,op,0.84,40\n",
					}
					scanFunc := func(args []interface{}) bool {
						if len(probabilitiesStr) == 0 {
							return false
						}
						for _, arg := range args {
							if ptr, ok := arg.(*string); ok {
								*ptr = probabilitiesStr[0]
								break
							}
						}
						probabilitiesStr = probabilitiesStr[1:]
						return true
					}
					return mock.MatchedBy(scanFunc)
				}

				iter := &mocks.Iterator{}
				iter.On("Scan", scanMatcher()).Return(true)
				iter.On("Scan", scanMatcher()).Return(false)
				iter.On("Close").Return(testCase.queryError)

				query := &mocks.Query{}
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				probabilities, err := s.store.GetLatestProbabilities()

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					assert.Equal(t, 0.84, probabilities["svc"]["op"])
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
			})
		})
	}
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}

func TestGenerateRandomBucket(t *testing.T) {
	assert.Contains(t, []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, generateRandomBucket())
}

func TestThroughputToString(t *testing.T) {
	throughput := []*model.Throughput{
		{Service: "svc1", Operation: "op,1", Count: 1, Probabilities: map[string]struct{}{"1": {}}},
		{Service: "svc2", Operation: "op2", Count: 2, Probabilities: map[string]struct{}{}},
	}
	str := throughputToString(throughput)
	assert.True(t, "svc1,\"op,1\",1,1\nsvc2,op2,2,\n" == str || "svc2,op2,2,\nsvc1,1\"op,1\",1,1\n" == str)

	throughput = []*model.Throughput{
		{Service: "svc1", Operation: "op,1", Count: 1, Probabilities: map[string]struct{}{"1": {}, "2": {}}},
	}
	str = throughputToString(throughput)
	assert.True(t, "svc1,\"op,1\",1,\"1,2\"\n" == str || "svc1,\"op,1\",1,\"2,1\"\n" == str)
}

func TestStringToThroughput(t *testing.T) {
	s := &SamplingStore{logger: zap.NewNop()}
	testStr := "svc1,\"op,1\",1,\"0.1,0.2\"\nsvc2,op2,2,\nsvc3,op3,string,\nsvc4,op4\nsvc5\n"
	throughput := s.stringToThroughput(testStr)

	assert.Len(t, throughput, 2)
	assert.Equal(t,
		model.Throughput{
			Service:       "svc1",
			Operation:     "op,1",
			Count:         1,
			Probabilities: map[string]struct{}{"0.1": {}, "0.2": {}}},
		*throughput[0],
	)
	assert.Equal(t,
		model.Throughput{
			Service:       "svc2",
			Operation:     "op2",
			Count:         2,
			Probabilities: map[string]struct{}{}},
		*throughput[1],
	)
}

func TestProbabilitiesAndQPSToString(t *testing.T) {
	probabilities := model.ServiceOperationProbabilities{
		"svc,1": map[string]float64{
			"GET": 0.001,
		},
	}
	qps := model.ServiceOperationQPS{
		"svc,1": map[string]float64{
			"GET": 62.3,
		},
	}
	str := probabilitiesAndQPSToString(probabilities, qps)
	assert.Equal(t, "\"svc,1\",GET,0.001,62.3\n", str)
}

func TestStringToProbabilitiesAndQPS(t *testing.T) {
	s := &SamplingStore{logger: zap.NewNop()}
	testStr := "svc1,GET,0.001,63.2\nsvc1,PUT,0.002,0.0\nsvc2,GET,0.5,34.2\nsvc2\nsvc2,PUT,string,22.2\nsvc2,DELETE,0.3,string\n"
	probabilities := s.stringToProbabilitiesAndQPS(testStr)

	assert.Len(t, probabilities, 2)
	assert.Equal(t, map[string]*model.ProbabilityAndQPS{"GET": {0.001, 63.2}, "PUT": {0.002, 0.0}}, probabilities["svc1"])
	assert.Equal(t, map[string]*model.ProbabilityAndQPS{"GET": {0.5, 34.2}}, probabilities["svc2"])
}

func TestStringToProbabilities(t *testing.T) {
	s := &SamplingStore{logger: zap.NewNop()}
	testStr := "svc1,GET,0.001,63.2\nsvc1,PUT,0.002,0.0\nsvc2,GET,0.5,34.2\nsvc2\nsvc2,PUT,string,34.2\n"
	probabilities := s.stringToProbabilities(testStr)

	assert.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.001, "PUT": 0.002}, probabilities["svc1"])
	assert.Equal(t, map[string]float64{"GET": 0.5}, probabilities["svc2"])
}

func TestProbabilitiesSetToString(t *testing.T) {
	s := probabilitiesSetToString(map[string]struct{}{"0.000001": {}, "0.000002": {}})
	assert.True(t, "0.000001,0.000002" == s || "0.000002,0.000001" == s)
	assert.Equal(t, "", probabilitiesSetToString(nil))
}

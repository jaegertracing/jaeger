// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var testTime = time.Date(2017, time.January, 24, 11, 15, 17, 12345, time.UTC)

type samplingStoreTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	store     *SamplingStore
}

func withSamplingStore(fn func(r *samplingStoreTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	r := &samplingStoreTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		store:     New(session, metricsFactory, logger),
	}
	fn(r)
}

var _ samplingstore.Store = &SamplingStore{} // check API conformance

func TestInsertThroughput(t *testing.T) {
	withSamplingStore(func(s *samplingStoreTest) {
		query := &mocks.Query{}
		query.On("Exec").Return(nil)

		var args []any
		captureArgs := mock.MatchedBy(func(v []any) bool {
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
		require.NoError(t, err)

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

		var args []any
		captureArgs := mock.MatchedBy(func(v []any) bool {
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
		require.NoError(t, err)

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
			expectedError: "error reading throughput from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSamplingStore(func(s *samplingStoreTest) {
				scanMatcher := func() any {
					throughputStr := []string{
						"\"svc,withcomma\",\"op,withcomma\",40,\"0.1,\"\n",
						"svc,op,50,\n",
					}
					scanFunc := func(args []any) bool {
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
					require.NoError(t, err)
					assert.Len(t, throughput, 2)
					assert.Equal(t,
						model.Throughput{
							Service:       "svc,withcomma",
							Operation:     "op,withcomma",
							Count:         40,
							Probabilities: map[string]struct{}{"0.1": {}},
						},
						*throughput[0],
					)
					assert.Equal(t,
						model.Throughput{
							Service:       "svc",
							Operation:     "op",
							Count:         50,
							Probabilities: map[string]struct{}{},
						},
						*throughput[1],
					)
				} else {
					require.EqualError(t, err, testCase.expectedError)
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
			expectedError: "error reading probabilities from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSamplingStore(func(s *samplingStoreTest) {
				scanMatcher := func() any {
					probabilitiesStr := []string{
						"svc,op,0.84,40\n",
					}
					scanFunc := func(args []any) bool {
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
					require.NoError(t, err)
					assert.InDelta(t, 0.84, probabilities["svc"]["op"], 0.01)
				} else {
					require.EqualError(t, err, testCase.expectedError)
				}
			})
		})
	}
}

func matchEverything() any {
	return mock.MatchedBy(func(_ []any) bool { return true })
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
	assert.True(t, str == "svc1,\"op,1\",1,1\nsvc2,op2,2,\n" || str == "svc2,op2,2,\nsvc1,1\"op,1\",1,1\n")

	throughput = []*model.Throughput{
		{Service: "svc1", Operation: "op,1", Count: 1, Probabilities: map[string]struct{}{"1": {}, "2": {}}},
	}
	str = throughputToString(throughput)
	assert.True(t, str == "svc1,\"op,1\",1,\"1,2\"\n" || str == "svc1,\"op,1\",1,\"2,1\"\n")
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
			Probabilities: map[string]struct{}{"0.1": {}, "0.2": {}},
		},
		*throughput[0],
	)
	assert.Equal(t,
		model.Throughput{
			Service:       "svc2",
			Operation:     "op2",
			Count:         2,
			Probabilities: map[string]struct{}{},
		},
		*throughput[1],
	)
}

func TestProbabilitiesAndQPSToString(t *testing.T) {
	probabilities := model.ServiceOperationProbabilities{
		"svc,1": map[string]float64{
			http.MethodGet: 0.001,
		},
	}
	qps := model.ServiceOperationQPS{
		"svc,1": map[string]float64{
			http.MethodGet: 62.3,
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
	assert.Equal(t, map[string]*probabilityAndQPS{
		http.MethodGet: {
			Probability: 0.001,
			QPS:         63.2,
		},
		http.MethodPut: {
			Probability: 0.002,
			QPS:         0.0,
		},
	}, probabilities["svc1"])
	assert.Equal(t, map[string]*probabilityAndQPS{
		http.MethodGet: {
			Probability: 0.5,
			QPS:         34.2,
		},
	}, probabilities["svc2"])
}

func TestStringToProbabilities(t *testing.T) {
	s := &SamplingStore{logger: zap.NewNop()}
	testStr := "svc1,GET,0.001,63.2\nsvc1,PUT,0.002,0.0\nsvc2,GET,0.5,34.2\nsvc2\nsvc2,PUT,string,34.2\n"
	probabilities := s.stringToProbabilities(testStr)

	assert.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.001, http.MethodPut: 0.002}, probabilities["svc1"])
	assert.Equal(t, map[string]float64{http.MethodGet: 0.5}, probabilities["svc2"])
}

func TestProbabilitiesSetToString(t *testing.T) {
	s := probabilitiesSetToString(map[string]struct{}{"0.000001": {}, "0.000002": {}})
	assert.True(t, s == "0.000001,0.000002" || s == "0.000002,0.000001")
	assert.Empty(t, probabilitiesSetToString(nil))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

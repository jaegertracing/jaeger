// Copyright (c) 2017 Uber Technologies, Inc.
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

package samplingstore

import (
	"bytes"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
)

const (
	buckets        = `(0,1,2,3,4,5,6,7,8,9)`
	constBucket    = 1
	constBucketStr = `1`

	insertThroughput    = `INSERT INTO operation_throughput(bucket, ts, throughput) VALUES (?, ?, ?)`
	getThroughput       = `SELECT throughput FROM operation_throughput WHERE bucket IN ` + buckets + ` AND ts > ? AND ts <= ?`
	insertProbabilities = `INSERT INTO sampling_probabilities(bucket, ts, hostname, probabilities) VALUES (?, ?, ?, ?)`
	getProbabilities    = `SELECT probabilities, hostname FROM sampling_probabilities WHERE bucket = ` + constBucketStr +
		` AND ts > ? AND ts <= ?`
	getLatestProbabilities = `SELECT probabilities FROM sampling_probabilities WHERE bucket = ` + constBucketStr + ` LIMIT 1`
)

type samplingStoreMetrics struct {
	operationThroughput *casMetrics.Table
	probabilities       *casMetrics.Table
}

// SamplingStore handles all insertions and queries for sampling data to and from Cassandra
type SamplingStore struct {
	session cassandra.Session
	metrics samplingStoreMetrics
	logger  *zap.Logger
}

// New creates a new cassandra sampling store.
func New(session cassandra.Session, factory metrics.Factory, logger *zap.Logger) *SamplingStore {
	return &SamplingStore{
		session: session,
		metrics: samplingStoreMetrics{
			operationThroughput: casMetrics.NewTable(factory, "OperationThroughput"),
			probabilities:       casMetrics.NewTable(factory, "Probabilities"),
		},
		logger: logger,
	}
}

// InsertThroughput implements samplingstore.Writer#InsertThroughput.
func (s *SamplingStore) InsertThroughput(throughput []*model.Throughput) error {
	throughputStr := throughputToString(throughput)
	query := s.session.Query(insertThroughput, generateRandomBucket(), gocql.TimeUUID(), throughputStr)
	return s.metrics.operationThroughput.Exec(query, s.logger)
}

// GetThroughput implements samplingstore.Reader#GetThroughput.
func (s *SamplingStore) GetThroughput(start, end time.Time) ([]*model.Throughput, error) {
	iter := s.session.Query(getThroughput, gocql.UUIDFromTime(start), gocql.UUIDFromTime(end)).Iter()
	var throughput []*model.Throughput
	var throughputStr string
	for iter.Scan(&throughputStr) {
		throughput = append(throughput, s.stringToThroughput(throughputStr)...)
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading throughput from storage")
		return nil, err
	}
	return throughput, nil
}

// InsertProbabilitiesAndQPS implements samplingstore.Writer#InsertProbabilitiesAndQPS.
func (s *SamplingStore) InsertProbabilitiesAndQPS(
	hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	probabilitiesAndQPSStr := probabilitiesAndQPSToString(probabilities, qps)
	query := s.session.Query(insertProbabilities, constBucket, gocql.TimeUUID(), hostname, probabilitiesAndQPSStr)
	return s.metrics.probabilities.Exec(query, s.logger)
}

// GetLatestProbabilities implements samplingstore.Reader#GetLatestProbabilities.
func (s *SamplingStore) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	iter := s.session.Query(getLatestProbabilities).Iter()
	var probabilitiesStr string
	iter.Scan(&probabilitiesStr)
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading probabilities from storage")
		return nil, err
	}
	return s.stringToProbabilities(probabilitiesStr), nil
}

// GetProbabilitiesAndQPS implements samplingstore.Reader#GetProbabilitiesAndQPS.
func (s *SamplingStore) GetProbabilitiesAndQPS(start, end time.Time) (map[string][]model.ServiceOperationData, error) {
	iter := s.session.Query(getProbabilities, gocql.UUIDFromTime(start), gocql.UUIDFromTime(end)).Iter()
	hostProbabilitiesAndQPS := make(map[string][]model.ServiceOperationData)
	var probabilitiesAndQPSStr, host string
	for iter.Scan(&probabilitiesAndQPSStr, &host) {
		hostProbabilitiesAndQPS[host] = append(hostProbabilitiesAndQPS[host], s.stringToProbabilitiesAndQPS(probabilitiesAndQPSStr))
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading probabilities and qps from storage")
		return nil, err
	}
	return hostProbabilitiesAndQPS, nil
}

// This is random enough for storage purposes
func generateRandomBucket() int64 {
	return time.Now().UnixNano() % 10
}

func probabilitiesAndQPSToString(probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) string {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for svc, opProbabilities := range probabilities {
		for op, probability := range opProbabilities {
			opQPS := 0.0
			if _, ok := qps[svc]; ok {
				opQPS = qps[svc][op]
			}
			writer.Write([]string{svc, op, strconv.FormatFloat(probability, 'f', -1, 64),
				strconv.FormatFloat(opQPS, 'f', -1, 64)})
		}
	}
	writer.Flush()
	return buf.String()
}

func (s *SamplingStore) stringToProbabilitiesAndQPS(probabilitiesAndQPSStr string) model.ServiceOperationData {
	probabilitiesAndQPS := make(model.ServiceOperationData)
	appendFunc := s.appendProbabilityAndQPS(probabilitiesAndQPS)
	s.parseString(probabilitiesAndQPSStr, 4, appendFunc)
	return probabilitiesAndQPS
}

func (s *SamplingStore) stringToProbabilities(probabilitiesStr string) model.ServiceOperationProbabilities {
	probabilities := make(model.ServiceOperationProbabilities)
	appendFunc := s.appendProbability(probabilities)
	s.parseString(probabilitiesStr, 4, appendFunc)
	return probabilities
}

func throughputToString(throughput []*model.Throughput) string {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, t := range throughput {
		writer.Write([]string{t.Service, t.Operation, strconv.Itoa(int(t.Count)), probabilitiesSetToString(t.Probabilities)})
	}
	writer.Flush()
	return buf.String()
}

func probabilitiesSetToString(probabilities map[string]struct{}) string {
	var buf bytes.Buffer
	for probability := range probabilities {
		buf.WriteString(probability)
		buf.WriteString(",")
	}
	return strings.TrimSuffix(buf.String(), ",")
}

func (s *SamplingStore) stringToThroughput(throughputStr string) []*model.Throughput {
	var throughput []*model.Throughput
	appendFunc := s.appendThroughput(&throughput)
	s.parseString(throughputStr, 4, appendFunc)
	return throughput
}

func (s *SamplingStore) appendProbabilityAndQPS(svcOpData model.ServiceOperationData) func(csvFields []string) {
	return func(csvFields []string) {
		probability, err := strconv.ParseFloat(csvFields[2], 64)
		if err != nil {
			s.logger.Warn("probability cannot be parsed", zap.Any("entries", csvFields), zap.Error(err))
			return
		}
		qps, err := strconv.ParseFloat(csvFields[3], 64)
		if err != nil {
			s.logger.Warn("qps cannot be parsed", zap.Any("entries", csvFields), zap.Error(err))
			return
		}
		service := csvFields[0]
		operation := csvFields[1]
		if _, ok := svcOpData[service]; !ok {
			svcOpData[service] = make(map[string]*model.ProbabilityAndQPS)
		}
		svcOpData[service][operation] = &model.ProbabilityAndQPS{
			Probability: probability,
			QPS:         qps,
		}
	}
}

func (s *SamplingStore) appendProbability(probabilities model.ServiceOperationProbabilities) func(csvFields []string) {
	return func(csvFields []string) {
		probability, err := strconv.ParseFloat(csvFields[2], 64)
		if err != nil {
			s.logger.Warn("probability cannot be parsed", zap.Any("entries", csvFields), zap.Error(err))
			return
		}
		service := csvFields[0]
		operation := csvFields[1]
		if _, ok := probabilities[service]; !ok {
			probabilities[service] = make(map[string]float64)
		}
		probabilities[service][operation] = probability
	}
}

func (s *SamplingStore) appendThroughput(throughput *[]*model.Throughput) func(csvFields []string) {
	return func(csvFields []string) {
		count, err := strconv.Atoi(csvFields[2])
		if err != nil {
			s.logger.Warn("throughput count cannot be parsed", zap.Any("entries", csvFields), zap.Error(err))
			return
		}
		*throughput = append(*throughput, &model.Throughput{
			Service:       csvFields[0],
			Operation:     csvFields[1],
			Count:         int64(count),
			Probabilities: parseProbabilitiesSet(csvFields[3]),
		})
	}
}

func parseProbabilitiesSet(probabilitiesStr string) map[string]struct{} {
	ret := map[string]struct{}{}
	for _, probability := range strings.Split(probabilitiesStr, ",") {
		if probability != "" {
			ret[probability] = struct{}{}
		}
	}
	return ret
}

func (s *SamplingStore) parseString(str string, numColumns int, appendFunc func(csvFields []string)) {
	reader := csv.NewReader(strings.NewReader(str))
	for {
		csvFields, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.logger.Error("failed to read csv", zap.Error(err))
		}
		if len(csvFields) != numColumns {
			s.logger.Warn("incomplete throughput data", zap.Int("expected_columns", numColumns), zap.Any("entries", csvFields))
			continue
		}
		appendFunc(csvFields)
	}
}

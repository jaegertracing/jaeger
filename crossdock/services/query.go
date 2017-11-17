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

package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	ui "github.com/jaegertracing/jaeger/model/json"
)

// QueryService is the service used to query cassandra tables for traces
type QueryService interface {
	GetTraces(serviceName, operation string, tags map[string]string) ([]*ui.Trace, error)
}

type queryService struct {
	url    string
	logger *zap.Logger
}

// NewQueryService returns an instance of QueryService.
func NewQueryService(url string, logger *zap.Logger) QueryService {
	return &queryService{
		url:    url,
		logger: logger,
	}
}

func getTraceURL(url string) string {
	return url + "/api/traces?%s"
}

type response struct {
	Data []*ui.Trace `json:"data"`
}

// GetTraces retrieves traces from the query service
func (s *queryService) GetTraces(serviceName, operation string, tags map[string]string) ([]*ui.Trace, error) {
	endTimeMicros := time.Now().Unix() * int64(time.Second/time.Microsecond)
	values := url.Values{}
	values.Add("service", serviceName)
	values.Add("operation", operation)
	values.Add("end", strconv.FormatInt(endTimeMicros, 10))
	for k, v := range tags {
		values.Add("tag", k+":"+v)
	}
	url := fmt.Sprintf(getTraceURL(s.url), values.Encode())
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	s.logger.Info("Retrieved trace from query", zap.String("body", string(body)), zap.String("url", url))

	var queryResponse response
	if err = json.Unmarshal(body, &queryResponse); err != nil {
		return nil, err
	}
	return queryResponse.Data, nil
}

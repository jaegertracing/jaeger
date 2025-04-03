// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	ui "github.com/jaegertracing/jaeger/internal/uimodel"
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
func NewQueryService(serviceURL string, logger *zap.Logger) QueryService {
	return &queryService{
		url:    serviceURL,
		logger: logger,
	}
}

func getTraceURL(traceURL string) string {
	return traceURL + "/api/traces?%s"
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
	fmtURL := fmt.Sprintf(getTraceURL(s.url), values.Encode())
	resp, err := http.Get(fmtURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	s.logger.Info("GetTraces: received response from query", zap.String("body", string(body)), zap.String("url", fmtURL))

	var queryResponse response
	if err = json.Unmarshal(body, &queryResponse); err != nil {
		return nil, err
	}
	return queryResponse.Data, nil
}

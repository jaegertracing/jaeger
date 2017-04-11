package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"time"

	ui "github.com/uber/jaeger/model/json"
	"go.uber.org/zap"
)

const (
	queryService = "Query"

	queryServiceURL = "http://127.0.0.1:16686"

	queryCmd = "/go/cmd/jaeger-query %s &"
)

// QueryService is the service used to query cassandra tables for traces
type QueryService struct {
	url    string
	logger *zap.Logger
}

// NewQueryService initiates the query service
func NewQueryService(url string, logger *zap.Logger) *QueryService {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf(queryCmd,
		"-cassandra.keyspace=jaeger -cassandra.servers=cassandra -cassandra.connections-per-host=1"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize query service", zap.Error(err))
	}
	if url == "" {
		url = queryServiceURL
	}
	healthCheck(logger, queryService, url)
	return &QueryService{url: url, logger: logger}
}

func getTraceURL(url string) string {
	return url + "/api/traces?%s"
}

type response struct {
	Data []*ui.Trace `json:"data"`
}

// GetTraces retrieves traces from the query service
func (s QueryService) GetTraces(serviceName, operation string, tags map[string]string) ([]*ui.Trace, error) {
	endTimeMicros := time.Now().Unix() * int64(time.Second/time.Microsecond)
	values := url.Values{}
	values.Add("service", serviceName)
	values.Add("operation", operation)
	values.Add("end", strconv.FormatInt(endTimeMicros, 10))
	for k, v := range tags {
		values.Add("tag", k+":"+v)
	}
	resp, err := http.Get(fmt.Sprintf(getTraceURL(s.url), values.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	s.logger.Info("Retrieved trace from query", zap.String("body", string(body)))

	var queryResponse response
	if err = json.Unmarshal(body, &queryResponse); err != nil {
		return nil, err
	}
	return queryResponse.Data, nil
}

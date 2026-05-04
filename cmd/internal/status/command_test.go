// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"status\":\"Server available\"}"))
}

func unavailableHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("{\"status\":\"Server not available\"}"))
}

func TestReady(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(readyHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestOnlyPortConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(readyHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=:" + strings.Split(ts.URL, ":")[len(strings.Split(ts.URL, ":"))-1]})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestUnready(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(unavailableHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestNoService(t *testing.T) {
	v := viper.New()
	cmd := Command(v, 12345)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestBodyReadFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}
		conn, buf, _ := hj.Hijack()
		buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\npartial")
		buf.Flush()
		conn.Close()
	}))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
	err := cmd.Execute()
	require.ErrorContains(t, err, "failed to read status response")
}

func TestInvalidStatusURL(t *testing.T) {
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=%zzinvalid"})
	err := cmd.Execute()
	require.ErrorContains(t, err, "failed to create status request")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

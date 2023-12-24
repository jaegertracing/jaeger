// Copyright (c) 2019 The Jaeger Authors.
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

package es

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/es"
	escfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

type mockClientBuilder struct {
	err                 error
	createTemplateError error
}

func (m *mockClientBuilder) NewClient(_ *escfg.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error) {
	if m.err == nil {
		c := &mocks.Client{}
		tService := &mocks.TemplateCreateService{}
		tService.On("Body", mock.Anything).Return(tService)
		tService.On("Do", context.Background()).Return(nil, m.createTemplateError)
		c.On("CreateTemplate", mock.Anything).Return(tService)
		c.On("GetVersion").Return(uint(6))
		c.On("Close").Return(nil)
		return c, nil
	}
	return nil, m.err
}

func TestElasticsearchFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())

	f.newClientFn = (&mockClientBuilder{err: errors.New("made-up error")}).NewClient
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "failed to create primary Elasticsearch client: made-up error")

	f.archiveConfig.Enabled = true
	f.newClientFn = func(c *escfg.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error) {
		// to test archive storage error, pretend that primary client creation is successful
		// but override newClientFn so it fails for the next invocation
		f.newClientFn = (&mockClientBuilder{err: errors.New("made-up error2")}).NewClient
		return (&mockClientBuilder{}).NewClient(c, logger, metricsFactory)
	}
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "failed to create archive Elasticsearch client: made-up error2")

	f.newClientFn = (&mockClientBuilder{}).NewClient
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err := f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateArchiveSpanReader()
	require.NoError(t, err)

	_, err = f.CreateArchiveSpanWriter()
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestElasticsearchTagsFileDoNotExist(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{
		Tags: escfg.TagsAsFields{
			File: "fixtures/file-does-not-exist.txt",
		},
	}
	f.archiveConfig = &escfg.Configuration{}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	defer f.Close()
	r, err := f.CreateSpanWriter()
	require.Error(t, err)
	assert.Nil(t, r)
}

func TestElasticsearchILMUsedWithoutReadWriteAliases(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{
		UseILM: true,
	}
	f.archiveConfig = &escfg.Configuration{}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	defer f.Close()
	w, err := f.CreateSpanWriter()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Nil(t, w)

	r, err := f.CreateSpanReader()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Nil(t, r)
}

func TestTagKeysAsFields(t *testing.T) {
	tests := []struct {
		path          string
		include       string
		expected      []string
		errorExpected bool
	}{
		{
			path:          "fixtures/do_not_exists.txt",
			errorExpected: true,
		},
		{
			path:     "fixtures/tags_01.txt",
			expected: []string{"foo", "bar", "space"},
		},
		{
			path:     "fixtures/tags_02.txt",
			expected: nil,
		},
		{
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
		{
			expected: nil,
		},
		{
			path:     "fixtures/tags_01.txt",
			include:  "televators,eriatarka,thewidow",
			expected: []string{"foo", "bar", "space", "televators", "eriatarka", "thewidow"},
		},
		{
			path:     "fixtures/tags_02.txt",
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
	}

	for _, test := range tests {
		cfg := escfg.Configuration{
			Tags: escfg.TagsAsFields{
				File:    test.path,
				Include: test.include,
			},
		}

		tags, err := cfg.TagKeysAsFields()
		if test.errorExpected {
			require.Error(t, err)
			assert.Nil(t, tags)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, tags)
		}
	}
}

func TestCreateTemplateError(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{CreateIndexTemplates: true}
	f.archiveConfig = &escfg.Configuration{}
	f.newClientFn = (&mockClientBuilder{createTemplateError: errors.New("template-error")}).NewClient
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()
	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	require.Error(t, err, "template-error")
}

func TestILMDisableTemplateCreation(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{UseILM: true, UseReadWriteAliases: true, CreateIndexTemplates: true}
	f.archiveConfig = &escfg.Configuration{}
	f.newClientFn = (&mockClientBuilder{createTemplateError: errors.New("template-error")}).NewClient
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	defer f.Close()
	require.NoError(t, err)
	_, err = f.CreateSpanWriter()
	require.NoError(t, err) // as the createTemplate is not called, CreateSpanWriter should not return an error
}

func TestArchiveDisabled(t *testing.T) {
	f := NewFactory()
	f.archiveConfig = &escfg.Configuration{Enabled: false}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	w, err := f.CreateArchiveSpanWriter()
	assert.Nil(t, w)
	require.NoError(t, err)
	r, err := f.CreateArchiveSpanReader()
	assert.Nil(t, r)
	require.NoError(t, err)
}

func TestArchiveEnabled(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{}
	f.archiveConfig = &escfg.Configuration{Enabled: true}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	w, err := f.CreateArchiveSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, w)
	r, err := f.CreateArchiveSpanReader()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestInitFromOptions(t *testing.T) {
	f := NewFactory()
	o := Options{
		Primary: namespaceConfig{Configuration: escfg.Configuration{Servers: []string{"server"}}},
		others:  map[string]*namespaceConfig{"es-archive": {Configuration: escfg.Configuration{Servers: []string{"server2"}}}},
	}
	f.InitFromOptions(o)
	assert.Equal(t, o.GetPrimary(), f.primaryConfig)
	assert.Equal(t, o.Get(archiveNamespace), f.archiveConfig)
}

func TestPasswordFromFile(t *testing.T) {
	t.Run("primary client", func(t *testing.T) {
		f := NewFactory()
		testPasswordFromFile(t, f, f.getPrimaryClient, f.CreateSpanWriter)
	})

	t.Run("archive client", func(t *testing.T) {
		f2 := NewFactory()
		testPasswordFromFile(t, f2, f2.getArchiveClient, f2.CreateArchiveSpanWriter)
	})

	t.Run("load token error", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "does not exist")
		token, err := loadTokenFromFile(file)
		require.Error(t, err)
		assert.Equal(t, "", token)
	})
}

func testPasswordFromFile(t *testing.T, f *Factory, getClient func() es.Client, getWriter func() (spanstore.Writer, error)) {
	const (
		pwd1 = "first password"
		pwd2 = "second password"
		// and with user name
		upwd1 = "user:" + pwd1
		upwd2 = "user:" + pwd2
	)
	var authReceived sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request to fake ES server: %v", r)
		// epecting header in the form Authorization:[Basic OmZpcnN0IHBhc3N3b3Jk]
		h := strings.Split(r.Header.Get("Authorization"), " ")
		require.Len(t, h, 2)
		require.Equal(t, "Basic", h[0])
		authBytes, err := base64.StdEncoding.DecodeString(h[1])
		require.NoError(t, err, "header: %s", h)
		auth := string(authBytes)
		authReceived.Store(auth, auth)
		t.Logf("request to fake ES server contained auth=%s", auth)
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))

	f.primaryConfig = &escfg.Configuration{
		Servers:          []string{server.URL},
		LogLevel:         "debug",
		Username:         "user",
		PasswordFilePath: pwdFile,
		BulkSize:         -1, // disable bulk; we want immediate flush
	}
	f.archiveConfig = &escfg.Configuration{
		Enabled:          true,
		Servers:          []string{server.URL},
		LogLevel:         "debug",
		Username:         "user",
		PasswordFilePath: pwdFile,
		BulkSize:         -1, // disable bulk; we want immediate flush
	}
	require.NoError(t, f.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
	defer f.Close()

	writer, err := getWriter()
	require.NoError(t, err)
	span := &model.Span{
		Process: &model.Process{ServiceName: "foo"},
	}
	require.NoError(t, writer.WriteSpan(context.Background(), span))
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd1)
			return ok && pwd == upwd1
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the first password",
	)

	t.Log("replace password in the file")
	client1 := getClient()
	newPwdFile := filepath.Join(t.TempDir(), "pwd2")
	require.NoError(t, os.WriteFile(newPwdFile, []byte(pwd2), 0o600))
	require.NoError(t, os.Rename(newPwdFile, pwdFile))

	assert.Eventually(t,
		func() bool {
			client2 := getClient()
			return client1 != client2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to change for the new password",
	)

	require.NoError(t, writer.WriteSpan(context.Background(), span))
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd2)
			return ok && pwd == upwd2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the new password",
	)
}

func TestFactoryESClientsAreNil(t *testing.T) {
	f := &Factory{}
	assert.Nil(t, f.getPrimaryClient())
	assert.Nil(t, f.getArchiveClient())
}

func TestPasswordFromFileErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte("first password"), 0o600))

	f := NewFactory()
	f.primaryConfig = &escfg.Configuration{
		Servers:          []string{server.URL},
		LogLevel:         "debug",
		PasswordFilePath: pwdFile,
	}
	f.archiveConfig = &escfg.Configuration{
		Servers:          []string{server.URL},
		LogLevel:         "debug",
		PasswordFilePath: pwdFile,
	}

	logger, buf := testutils.NewEchoLogger(t)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	defer f.Close()

	f.primaryConfig.Servers = []string{}
	f.onPrimaryPasswordChange()
	assert.Contains(t, buf.String(), "no servers specified")

	f.archiveConfig.Servers = []string{}
	buf.Reset()
	f.onArchivePasswordChange()
	assert.Contains(t, buf.String(), "no servers specified")

	require.NoError(t, os.Remove(pwdFile))
	f.onPrimaryPasswordChange()
	f.onArchivePasswordChange()
}

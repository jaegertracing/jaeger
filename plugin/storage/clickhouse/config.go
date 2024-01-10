// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type Config struct {
	Endpoint       string `mapstructure:"endpoint"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	Database       string `mapstructure:"database"`
	SpansTableName string `mapstructure:"spans_table_name"`
	// Materialized views' names?
}

var driverName = "clickhouse"

const (
	defaultDatabase = "default"
	defaultUsername = "default"
	defaultPassword = ""
)

func (cfg *Config) NewClient(ctx context.Context) (*sql.DB, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("no endpoints specified")
	}

	dsnURL, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	queryParams := dsnURL.Query()

	if dsnURL.Scheme == "https" {
		queryParams.Set("secure", "true")
	}

	if cfg.Database == "" {
		cfg.Database = defaultDatabase
	}
	dsnURL.Path = cfg.Database

	if cfg.Username == "" {
		cfg.Username = defaultUsername
	}

	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}

	dsnURL.User = url.UserPassword(cfg.Username, cfg.Password)

	dsnURL.RawQuery = queryParams.Encode()

	dsn := dsnURL.String()

	if _, err = clickhouse.ParseDSN(dsn); err != nil {
		return nil, err
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	createDBquery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", cfg.Database)
	_, err = db.ExecContext(ctx, createDBquery)
	if err != nil {
		return nil, err
	}

	return db, nil
}

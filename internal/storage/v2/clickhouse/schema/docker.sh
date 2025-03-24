#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

clickhouse-client --host ${CLICKHOUSE_HOST} --user ${CLICKHOUSE_USERNAME} --password ${CLICKHOUSE_PASSWORD} --queries-file ${QUERY_FILE}
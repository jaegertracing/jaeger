# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM docker.io/bitnami/clickhouse:25

COPY schema/* /clickhouse-schema/

ENTRYPOINT ["sh","/clickhouse-schema/docker.sh"]
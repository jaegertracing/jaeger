# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM cassandra:5.0.7@sha256:3a926646888c7ff126d8a5795f7e03c6cb8e1566d3500bb680dcbed681f93647

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot nonroot --create-home

USER 65532:65532
ENTRYPOINT ["/cassandra-schema/docker.sh"]

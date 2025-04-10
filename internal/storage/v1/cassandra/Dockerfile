# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM cassandra:5.0.2@sha256:9945dafdc759800f1e129ee871e45c9d3aa304fb5149148bde8685ae9812b81b

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot nonroot --create-home

USER 65532:65532
ENTRYPOINT ["/cassandra-schema/docker.sh"]

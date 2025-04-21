# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM cassandra:5.0.4@sha256:648243f60690dc49f4ed7e7f8aceda1597ee3aacb427453a235f15480996676e

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot nonroot --create-home

USER 65532:65532
ENTRYPOINT ["/cassandra-schema/docker.sh"]

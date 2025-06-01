# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

FROM cassandra:5.0.4@sha256:ee5be67d740b5a427881effcfb672b6c986122ec139eada751f82bca247d6904

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra

RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot nonroot --create-home

USER 65532:65532
ENTRYPOINT ["/cassandra-schema/docker.sh"]

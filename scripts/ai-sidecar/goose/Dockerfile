# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This image runs a released Goose binary as the ACP agent of the stack in
# docker-compose.yml. Goose publishes no versioned container image, so the
# binary is taken from its GitHub release for the build platform.
FROM alpine:3.21
ARG GOOSE_VERSION=v1.57.0
ARG TARGETARCH
RUN apk add --no-cache ca-certificates curl \
  && case "$TARGETARCH" in \
       amd64) arch=x86_64; checksum=6c3a05e30e1a425dc06a410dbe24b3f3697c9721b1ea2340434ddda6b64267ae ;; \
       arm64) arch=aarch64; checksum=00a32522f0421ed72fc06f36bfc44c8e51cffefb4daf22a81bb84ccd29d1a1ce ;; \
       *) echo "unsupported arch $TARGETARCH"; exit 1 ;; \
     esac \
  && curl -fsSL "https://github.com/aaif-goose/goose/releases/download/${GOOSE_VERSION}/goose-${arch}-unknown-linux-musl.tar.gz" \
     -o /tmp/goose.tar.gz \
  && echo "$checksum  /tmp/goose.tar.gz" | sha256sum -c - \
  && tar -xz -C /usr/local/bin -f /tmp/goose.tar.gz ./goose \
  && rm /tmp/goose.tar.gz \
  && apk del curl
ENV HOME=/tmp
# Goose enables its bundled extensions (shell access, apps, skills and more)
# unless its config file disables each by name, and it has no switch that turns
# them all off. "goose info -v" prints the effective configuration as YAML under
# a "goose Configuration:" heading, so that section, with every extension
# flipped to disabled, becomes the config file. The agent then has only the
# tools Jaeger announces for the turn, and a new Goose version brings its own
# extension list.
RUN mkdir -p "$HOME/.config/goose" \
  && goose info -v \
     | sed -n '/^goose Configuration:/,$p' \
     | sed '1d; s/^  //; s/enabled: true/enabled: false/' \
     > "$HOME/.config/goose/config.yaml" \
  && grep -q "enabled: false" "$HOME/.config/goose/config.yaml" \
  && ! grep -q "enabled: true" "$HOME/.config/goose/config.yaml" \
  && cat "$HOME/.config/goose/config.yaml"
EXPOSE 16688
ENTRYPOINT ["goose", "serve", "--host", "0.0.0.0", "--port", "16688"]

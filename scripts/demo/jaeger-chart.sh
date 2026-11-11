#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# renovate: datasource=git-refs depName=https://github.com/jaegertracing/helm-charts.git
JAEGER_CHART_REF="v2"
JAEGER_CHART_COMMIT="28017780d96bd42f371bef635e2d4bbf938170e7"

prepare_jaeger_chart() {
  local dest=$1
  if [[ ! -d "$dest/.git" ]]; then
    git clone https://github.com/jaegertracing/helm-charts.git "$dest"
  fi
  git -C "$dest" fetch origin "$JAEGER_CHART_REF"
  git -C "$dest" checkout --detach "$JAEGER_CHART_COMMIT"
  helm repo add bitnami https://charts.bitnami.com/bitnami
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo add incubator https://charts.helm.sh/incubator
  helm repo update
  helm dependency build "$dest/charts/jaeger"
}

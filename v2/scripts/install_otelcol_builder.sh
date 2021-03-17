#!/usr/bin/env bash

while getopts d:v: flag
do
    case "${flag}" in
        d) otelcol_builder_dir=${OPTARG};;
        v) otelcol_builder_version=${OPTARG};;
    esac
done

if [ "$(which opentelemetry-collector-builder)" ]; then
  # Hacky, update once https://github.com/open-telemetry/opentelemetry-collector-builder/issues/5 is fixed
  current_builder_version=$(opentelemetry-collector-builder --help | grep "builder (" | sed 's/^.*(//g' | sed 's/)//g')
  if [ "${otelcol_builder_version}" == "${current_builder_version}" ]; then
    echo "opentelemetry-collector-builder ${current_builder_version} already installed"
    exit 0
  else
    echo "found old version of opentelemetry-collector-builder (${current_builder_version}), deleting"
    rm -rf $(which opentelemetry-collector-builder)
  fi
fi

echo "installing opentelemetry-collector-builder"
otelcol_builder="$otelcol_builder_dir/opentelemetry-collector-builder"
goos=$(go env GOOS)
goarch=$(go env GOARCH)

set -ex
mkdir -p "$otelcol_builder_dir"
curl -sLo "$otelcol_builder" "https://github.com/open-telemetry/opentelemetry-collector-builder/releases/download/v${otelcol_builder_version}/opentelemetry-collector-builder_${otelcol_builder_version}_${goos}_${goarch}"
chmod +x "${otelcol_builder}"

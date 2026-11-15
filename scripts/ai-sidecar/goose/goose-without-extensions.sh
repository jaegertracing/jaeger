#!/bin/sh
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script runs Goose with its bundled extensions (shell access, apps,
# skills and more) disabled, so that an agent started through it has only the
# tools Jaeger announces for the turn. It is the command of a container that
# runs the image Goose publishes, and it passes its arguments to goose:
#
#   sh goose-without-extensions.sh serve --host 0.0.0.0 --port 16688
#
# Goose enables those extensions unless its config file disables each by name,
# and it has no switch that turns them all off. "goose info -v" prints the
# effective configuration as YAML under a "goose Configuration:" heading, so
# that section, with every extension flipped to disabled, becomes the config
# file, and a new Goose version brings its own extension list.
set -eu

config="$HOME/.config/goose/config.yaml"
mkdir -p "$(dirname "$config")"
goose info -v \
  | sed -n '/^goose Configuration:/,$p' \
  | sed '1d; s/^  //; s/enabled: true/enabled: false/' \
  > "$config"
if ! grep -q "enabled: false" "$config" || grep -q "enabled: true" "$config"; then
  echo "FAIL: could not disable Goose's bundled extensions; $config holds:" >&2
  cat "$config" >&2
  exit 1
fi
exec goose "$@"

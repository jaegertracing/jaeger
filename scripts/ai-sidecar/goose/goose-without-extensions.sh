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
# Goose has no switch that turns the extensions off, only an entry for each in
# its config file; https://github.com/aaif-goose/goose/issues/11728 asks for
# one, and this script reduces to passing it once it exists. The file is
# generated from "goose info -v" and not checked in, because each Goose version
# has its own extension list and a fixed list would leave a new extension
# enabled.
set -eu

# The report is captured before the config file is opened for writing, so that
# Goose reads the settings of an existing file before the file is truncated.
report=$(goose info -v)
config=$(printf '%s\n' "$report" | sed -n 's/^Config yaml: *\(.*\/config\.yaml\).*/\1/p')
if [ -z "$config" ]; then
  echo "FAIL: \"goose info -v\" did not report the path of the Goose config file" >&2
  exit 1
fi
mkdir -p "$(dirname "$config")"
printf '%s\n' "$report" \
  | sed -n '/^goose Configuration:/,$p' \
  | sed '1d; s/^  //; s/enabled: true/enabled: false/' \
  > "$config"
# The check asks Goose again, so that it covers the configuration Goose loads
# and not only the file written above.
loaded=$(goose info -v)
if ! printf '%s\n' "$loaded" | grep -q "enabled: false" || printf '%s\n' "$loaded" | grep -q "enabled: true"; then
  echo "FAIL: Goose does not report every bundled extension as disabled:" >&2
  printf '%s\n' "$loaded" >&2
  exit 1
fi
exec goose "$@"

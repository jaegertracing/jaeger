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
#
# The script owns the Goose config file. It is written for a container that
# brings no config file of its own and configures Goose through environment
# variables, so it writes the file at the default path under the home directory
# and replaces it on every start.
set -eu

config="$HOME/.config/goose/config.yaml"
report=$(goose info -v)
mkdir -p "$(dirname "$config")"
# The report prints the configuration as YAML after a heading, indented by two
# spaces. Only the entries of its "extensions" section are changed and checked,
# because the report holds whatever else Goose chooses to put in it, and
# another section may have an "enabled" setting of its own.
printf '%s\n' "$report" | awk '
  /^goose Configuration:/ { in_config = 1; next }
  !in_config              { next }
                          { sub(/^  /, "") }
  /^[^ ]/                 { in_extensions = ($0 == "extensions:") }
  in_extensions           { sub(/enabled: true$/, "enabled: false") }
                          { print }
' > "$config"
# The check asks Goose again, so that it covers the configuration Goose loads
# and not only the file written above.
loaded_extensions=$(goose info -v | awk '
  /^goose Configuration:/ { in_config = 1; next }
  in_config && /^  [^ ]/  { in_extensions = ($0 == "  extensions:") }
  in_extensions           { print }
')
if ! printf '%s\n' "$loaded_extensions" | grep -q "enabled: false$" || printf '%s\n' "$loaded_extensions" | grep -q "enabled: true$"; then
  echo "FAIL: Goose does not report every bundled extension as disabled:" >&2
  printf '%s\n' "$loaded_extensions" >&2
  exit 1
fi
exec goose "$@"

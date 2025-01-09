#!/bin/bash
#
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

echo -n '{ "include": [ '
first="true"
for pair in $(make echo-platforms | tr ',' ' '); do
  os=$(echo "$pair" | cut -d '/' -f 1)
  arch=$(echo "$pair" | cut -d '/' -f 2)
  if [[ "$first" == "true" ]]; then
    first="false"
  else
    echo -n ' ,'
  fi
  echo -n "{ \"os\": \"$os\", \"arch\": \"$arch\" }"
done

echo "]}"

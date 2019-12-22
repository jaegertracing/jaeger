#!/bin/bash

# This script runs the `help` command on all Jaeger binaries (using go run) with variations of SPAN_STORAGE_TYPE.
# It can be used to compare the CLI API changes between releases.

dir=$1
if [[ "$dir" = "" ]]; then
  echo specify output dir
  exit 1
fi

function gen {
  bin=$1
  shift
  for s in "$@"
  do
    SPAN_STORAGE_TYPE=$s go run ./cmd/$bin help > $dir/$bin-$s.txt
  done
}

set -ex

gen agent      nostorage
gen collector  cassandra elasticsearch memory kafka badger grpc-plugin
gen query      cassandra elasticsearch memory badger grpc-plugin
gen ingester   cassandra elasticsearch memory badger grpc-plugin
gen all-in-one cassandra elasticsearch memory badger grpc-plugin

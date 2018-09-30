#!/bin/bash

set -euo pipefail

function listPackages() {
	local d
	local dirs
	dirs=$(find . -mindepth 1 -maxdepth 1 -type d \
		-not -path './vendor' \
		-not -path './thrift-gen' \
		-not -path './swagger-gen' \
		-not -path './examples' \
		-not -path './scripts')
	for d in $dirs; do
		find "$d" -name '*.go' -type f -exec echo "$d/..." \; -quit
	done
}

listPackages "$1"

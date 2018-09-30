#!/bin/bash

set -euo pipefail

function listPackages() {
	local d
	local dirs
	dirs=$(find -E . -mindepth 1 -maxdepth 1 -type d -not -regex '\./(vendor|thrift-gen|swagger-gen|examples|scripts)' | sed 's|^./||g')
	for d in $dirs; do
		find "$d" -name '*.go' -type f -exec echo "./$d/..." \; -quit | sed -e 's|^\./\./|./|g'
	done
}

listPackages "$1"

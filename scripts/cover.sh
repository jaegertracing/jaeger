#!/bin/bash

set -e

COVER=.cover
ROOT_PKG=github.com/jaegertracing/jaeger/

if [[ -d "$COVER" ]]; then
	rm -rf "$COVER"
fi
mkdir -p "$COVER"

# If a package directory has a .nocover file, don't count it when calculating
# coverage.
filter=""
for pkg in "$@"; do
	if [[ -f "$GOPATH/src/$pkg/.nocover" ]]; then
		if [[ -n "$filter" ]]; then
			filter="$filter, "
		fi
		filter="\"$pkg\": true"
	fi
done

i=0
for pkg in "$@"; do
	i=$((i + 1))

	extracoverpkg=""
	if [[ -f "$GOPATH/src/$pkg/.extra-coverpkg" ]]; then
		extracoverpkg=$( \
			sed -e "s|^|$pkg/|g" < "$GOPATH/src/$pkg/.extra-coverpkg" \
			| tr '\n' ',')
	fi

	coverpkg=$(go list -json "$pkg" | jq -r '
		.Deps
		| . + ["'"$pkg"'"]
		| map
			( select(startswith("'"$ROOT_PKG"'"))
			| select(contains("/vendor/") | not)
			| select(in({'"$filter"'}) | not)
			)
		| join(",")
	')
	if [[ -n "$extracoverpkg" ]]; then
		coverpkg="$extracoverpkg$coverpkg"
	fi

	args=""
	if [[ -n "$coverpkg" ]]; then
		args="-coverprofile $COVER/cover.${i}.out" # -coverpkg $coverpkg
	fi

	if [[ $(uname -m) == 's390x' ]]; then
		echo go test $args -v "$pkg"
		go test $args -v "$pkg"
	elif [[ $(uname -m) == 'ppc64le' ]]; then
		echo go test $args -v "$pkg"
		go test $args -v "$pkg"
	else
		echo go test $args -v -race "$pkg"
		go test $args -v -race "$pkg"
	fi
done

gocovmerge "$COVER"/*.out > cover.out

#!/bin/bash

set -euo pipefail

bad_pkgs=0

# shellcheck disable=SC2048
for dir in $*; do
  if [[ -f "${dir}/.nocover" ]]; then
    continue
  fi
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  if [[ -z "$testFiles" ]]; then
    continue
  fi 
  good=0
  for test in ${testFiles}; do
    if grep -q "TestMain" "${test}" | grep -q "testutils.VerifyGoLeaks" "${test}"; then
      good=1
      break
    fi
  done
  if ((good == 0)); then
    echo "🔴 Error(check-goleak): no goleak check in package ${dir}"
    ((bad_pkgs+=1))
  fi
done

if ((bad_pkgs > 0)); then
  echo "Error(check-goleak): no goleak check in ${bad_pkgs} package(s)."
  echo "See https://github.com/jaegertracing/jaeger/pull/5010/files for example of adding the checks."
  echo "In the future this will be a fatal error in the CI."
  exit 0 # TODO change to 1 in the future
fi

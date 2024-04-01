#!/bin/bash

set -euo pipefail

bad_pkgs=0
failed_pkgs=0

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
    if grep -q "TestMain" "${test}" && grep -q "testutils.VerifyGoLeaks" "${test}"; then
      good=1
      break
    fi
  done
  if ((good == 0)); then
    echo "🔴 Error(check-goleak): no goleak check in package ${dir}"
    ((bad_pkgs+=1))
    if [[ "${dir}" == "./cmd/jaeger/internal/integration/" || "${dir}" == "./plugin/storage/integration/" ]]; then
      echo "	this package is temporarily allowed and will not cause linter failure"
    else
      ((failed_pkgs+=1))
    fi
  fi
done

function help() {
  echo "	See https://github.com/jaegertracing/jaeger/pull/5010/files"
  echo "	for examples of adding the checks."
}

if ((failed_pkgs > 0)); then
  echo "⛔ Fatal(check-goleak): no goleak check in ${bad_pkgs} package(s), ${failed_pkgs} of which not allowed."
  help
  exit 1
elif ((bad_pkgs > 0)); then
  echo "🐞 Warning(check-goleak): no goleak check in ${bad_pkgs} package(s)."
  help
fi

#!/bin/bash

set -euo pipefail

NO_TEST_FILE_DIRS=""
for dir in $*; do
  mainFile=$(find ${dir} -depth 1 -name 'main.go')
  testFiles=$(find ${dir} -depth 1 -name '*_test.go')
  if [ -z "${testFiles}" ]; then
    if [ -n "${mainFile}" ]; then
      continue # single main does not require tests
    fi
    if [ -e ${dir}/.nocover ]; then
      reason=$(cat ${dir}/.nocover)
      if [ "${reason}" == "" ]; then 
        echo "error: ${dir}/.nocover must specify reason" >&2
        exit 1
      fi
      echo "Package excluded from coverage: ${dir}"
      echo "  reason: ${reason}"
      continue
    fi
    # if [ -e ${dir}/main.go ]; then
    #   continue
    # fi
    if [ -z "${NO_TEST_FILE_DIRS}" ]; then
      NO_TEST_FILE_DIRS="${dir}"
    else
      NO_TEST_FILE_DIRS="${NO_TEST_FILE_DIRS} ${dir}"
    fi
  fi
done

if [ -n "${NO_TEST_FILE_DIRS}" ]; then
  echo "error: at least one _test.go file must be in all directories with go files so that they are counted for code coverage:" >&2
  echo ${NO_TEST_FILE_DIRS} | tr ' ' '\n' >&2
  exit 1
fi

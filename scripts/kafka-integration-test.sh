#!/bin/bash

set -e

export STORAGE=kafka
while true; do
    if nc -z localhost 9092; then
        break
    fi
done
make storage-integration-test

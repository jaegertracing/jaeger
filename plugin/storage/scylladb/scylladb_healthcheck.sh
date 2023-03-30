#!/bin/bash

# Define the expected number of nodes
EXPECTED_NODES=3
SCYLLADB_CONTAINER_NAME=jaeger-scylladb-1

# Get the actual number nodes which status is UN (Up and Normal)
ACTUAL_NODES=$(docker exec -it $(docker ps -a | grep $SCYLLADB_CONTAINER_NAME | cut -d' ' -f1) nodetool status | grep '^UN' | wc -l)

# Compare the actual number of nodes with the expected number
if test "$ACTUAL_NODES" -ne "$EXPECTED_NODES"; then
  exit 1
fi

exit 0

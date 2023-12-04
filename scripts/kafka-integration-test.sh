#!/bin/bash

set -e

export STORAGE=kafka

# Function to start Kafka
start_kafka() {
    echo "Starting Kafka..."
    
    docker run --name kafka -d \
    -p 9092:9092 \
    -e KAFKA_CFG_NODE_ID=0 \
    -e KAFKA_CFG_PROCESS_ROLES=controller,broker \
    -e KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@localhost:9093 \
    -e KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
    -e KAFKA_CFG_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \
    -e KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
    -e KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER \
    -e KAFKA_CFG_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
    bitnami/kafka:3.6
}

# Check if the -k parameter is provided or not
if [ "$1" == "-k" ]; then
    start_kafka
fi

# Set the timeout in seconds
timeout=180
# Set the interval between checks in seconds
interval=5

# Calculate the end time
end_time=$((SECONDS + timeout))

while [ $SECONDS -lt $end_time ]; do
    if nc -z localhost 9092; then
        break
    fi
    sleep $interval
done

# Check if Kafka is still not available after the timeout
if ! nc -z localhost 9092; then
    echo "Timed out waiting for Kafka to start"
    exit 1
fi

make storage-integration-test
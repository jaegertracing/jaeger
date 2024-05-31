#!/bin/bash

set -e

export STORAGE=kafka
compose_file="docker-compose/kafka-integration-test/v3.yml"
# Check if the -k parameter is provided or not
if [ "$1" == "-k" ]; then
    echo "Starting Kafka using Docker Compose..."
    docker compose -f $compose_file up -d kafka
fi

# Set the timeout in seconds
timeout=180
# Set the interval between checks in seconds
interval=5

# Function to check if Kafka is ready
is_kafka_ready() {
    docker compose -f $compose_file exec kafka /opt/bitnami/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092 >/dev/null 2>&1
}

# Calculate the end time
end_time=$((SECONDS + timeout))

while [ $SECONDS -lt $end_time ]; do
    # Check if Kafka is ready by attempting to describe a topic
    if is_kafka_ready; then
        break
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
done

# Check if Kafka is still not available after the timeout
if ! is_kafka_ready; then
    echo "Timed out waiting for Kafka to start"
    exit 1
fi

# Continue with the integration tests
make storage-integration-test

#!/bin/bash

set -e

export STORAGE=kafka
compose_file="docker-compose/kafka-integration-test/v3.yml"

# Check if the -k parameter is provided and start Kafka if it was
if [ "$1" == "-k" ]; then
    echo "Starting Kafka using Docker Compose..."
    docker compose -f "${compose_file}" up -d kafka
    echo "docker_compose_file=${compose_file}" >> "${GITHUB_OUTPUT:-/dev/null}"
fi

# Check if Kafka is ready by attempting to list topics
is_kafka_ready() {
    docker compose -f "${compose_file}" \
        exec kafka /opt/bitnami/kafka/bin/kafka-topics.sh \
        --list \
        --bootstrap-server localhost:9092 \
        >/dev/null 2>&1
}

# Set the timeout in seconds
timeout=180
# Set the interval between checks in seconds
interval=5
# Calculate the end time
end_time=$((SECONDS + timeout))

while [ $SECONDS -lt $end_time ]; do
    if is_kafka_ready; then
        break
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
done

if ! is_kafka_ready; then
    echo "Timed out waiting for Kafka to start"
    exit 1
fi

make storage-integration-test

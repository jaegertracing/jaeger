#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

compose_file=""
jaeger_version="v2"
kafka_version="v3"
manage_kafka="true"
success="false"
# Store certificate paths for cleanup
CERTS_DIR=""
TEST_CERTS_DIR=""

usage() {
  echo "Usage: $0 [-S] [-j <jaeger_version>] [-v <kafka_version>]"
  echo "  -S: 'no storage' - do not start or stop Kafka container (useful for local testing)"
  echo "  -j: major version of Jaeger to test (v1|v2); default: v2"
  echo "  -v: kafka major version (3.x); default: 3.x"
  exit 1
}

parse_args() {
  while getopts "j:v:Sh" opt; do
    case "${opt}" in
    j)
      jaeger_version=${OPTARG}
      ;;
    v)
      case ${OPTARG} in
      3.x)
        kafka_version="v3"
        ;;
      2.x)
        kafka_version="v2"
        ;;
      *)
        echo "Error: Invalid Kafka version. Valid options are 3.x or 2.x"
        usage
        ;;
      esac
      ;;
    S)
      manage_kafka="false"
      ;;
    *)
      usage
      ;;
    esac
  done
  if [[ "$jaeger_version" != "v1" && "$jaeger_version" != "v2" ]]; then
    echo "Error: Invalid Jaeger version. Valid options are v1 or v2"
    usage
  fi
  compose_file="docker-compose/kafka/${kafka_version}/docker-compose.yml"
}

generate_kafka_certificates() {
  echo "Generating Kafka certificates..."
  
  # Get the absolute path of the script directory
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  
  # Define the target directory for certificates
  CERTS_DIR="${SCRIPT_DIR}/../../docker-compose/kafka/certs"
  TEST_CERTS_DIR="${SCRIPT_DIR}/../../certs"
  
  mkdir -p "$CERTS_DIR"
  cd "$CERTS_DIR"
  
  echo "Generating certificates in: $CERTS_DIR"
  
  # Create a configuration file for OpenSSL with more comprehensive SANs
  cat > openssl.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = kafka
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = localhost
DNS.2 = kafka
DNS.3 = 127.0.0.1
DNS.4 = *.kafka
DNS.5 = kafka.default
DNS.6 = kafka.default.svc.cluster.local
IP.1 = 127.0.0.1
EOF
  
  # Generate CA
  openssl req -new -x509 -keyout ca-key -out ca-cert -days 3650 -subj "/CN=Kafka-CA" -nodes
  
  # Generate keystore with the correct hostname
  keytool -keystore kafka.keystore.jks -alias kafka -validity 3650 -genkey \
      -keyalg RSA -storepass kafkapass123 -keypass kafkapass123 \
      -dname "CN=kafka" \
      -ext san=dns:localhost,dns:kafka,dns:kafka.default,dns:kafka.default.svc.cluster.local,ip:127.0.0.1
  
  # Create CSR from keystore with SAN extension
  keytool -keystore kafka.keystore.jks -alias kafka -certreq -file cert-file \
      -storepass kafkapass123 -keypass kafkapass123 \
      -ext san=dns:localhost,dns:kafka,dns:kafka.default,dns:kafka.default.svc.cluster.local,ip:127.0.0.1
  
  # Sign the CSR with our CA, including the SAN extension
  openssl x509 -req -CA ca-cert -CAkey ca-key -in cert-file -out cert-signed \
      -days 3650 -CAcreateserial -passin pass:kafkapass123 \
      -extfile openssl.cnf -extensions v3_req
  
  # Import CA into keystore
  keytool -keystore kafka.keystore.jks -alias CARoot -import -file ca-cert \
      -storepass kafkapass123 -keypass kafkapass123 -noprompt
  
  # Import signed certificate into keystore
  keytool -keystore kafka.keystore.jks -alias kafka -import -file cert-signed \
      -storepass kafkapass123 -keypass kafkapass123 -noprompt
  
  # Create truststore and import the CA
  keytool -keystore kafka.truststore.jks -alias CARoot -import -file ca-cert \
      -storepass kafkapass123 -keypass kafkapass123 -noprompt
  
  # Export the CA certificate in PEM format for clients that need it
  openssl x509 -in ca-cert -out ca.pem -outform PEM
  
  # Also copy the CA certificate to the location expected by the tests
  mkdir -p "$TEST_CERTS_DIR"
  cp ca.pem "$TEST_CERTS_DIR/"
  echo "Also copied ca.pem to $TEST_CERTS_DIR for tests"
  
  # Return to the original directory
  cd - > /dev/null
  
  echo "Certificates generated successfully!"
}

cleanup_certificates() {
  echo "Cleaning up certificates..."
  
  if [[ -n "$CERTS_DIR" && -d "$CERTS_DIR" ]]; then
    echo "Removing certificates from $CERTS_DIR"
    rm -rf "$CERTS_DIR"
  fi
  
  if [[ -n "$TEST_CERTS_DIR" && -d "$TEST_CERTS_DIR" ]]; then
    echo "Removing test certificates from $TEST_CERTS_DIR"
    rm -rf "$TEST_CERTS_DIR"
  fi
  
  echo "Certificate cleanup completed"
}

setup_kafka() {
  echo "Starting Kafka using Docker Compose..."
  docker compose -f "${compose_file}" up -d kafka
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 Kafka logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

teardown_kafka() {
  if [[ "$success" == "false" ]]; then
    dump_logs
  fi
  echo "Stopping Kafka..."
  docker compose -f "${compose_file}" down
  
  # Clean up certificates
  cleanup_certificates
}

is_kafka_ready() {
  docker compose -f "${compose_file}" \
    exec kafka /opt/bitnami/kafka/bin/kafka-topics.sh \
    --list \
    --bootstrap-server localhost:9092 \
    >/dev/null 2>&1
}

wait_for_kafka() {
  local timeout=180
  local interval=5
  local end_time=$((SECONDS + timeout))

  while [ $SECONDS -lt $end_time ]; do
    if is_kafka_ready; then
      return
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
  done

  echo "Timed out waiting for Kafka to start"
  exit 1
}

run_integration_test() {
  export STORAGE=kafka
  if [ "${jaeger_version}" = "v1" ]; then
    make storage-integration-test
  elif [ "${jaeger_version}" = "v2" ]; then
    make jaeger-v2-storage-integration-test
  else
    echo "Unknown Jaeger version ${jaeger_version}."
    usage
  fi
}

main() {
  parse_args "$@"

  echo "Executing Kafka integration test."
  echo "Kafka version ${kafka_version}."
  echo "Jaeger version ${jaeger_version}."
  set -x

  # Generate certificates before starting Kafka
  generate_kafka_certificates

  if [[ "$manage_kafka" == "true" ]]; then
    setup_kafka
    trap 'teardown_kafka' EXIT
  fi
  wait_for_kafka

  run_integration_test

  success="true"
}

main "$@"

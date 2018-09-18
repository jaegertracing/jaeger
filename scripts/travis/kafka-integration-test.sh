#!/bin/bash

set -e

function remove_containers() {
  docker rm -f kafka
  docker rm -f zookeeper
}

docker pull wurstmeister/kafka
docker pull zookeeper
docker run -d --rm --name zookeeper zookeeper
docker run -d --rm --link zookeeper --name kafka -p 9092:9092 -e KAFKA_ZOOKEEPER_CONNECT=zookeeper -e KAFKA_LISTENERS=PLAINTEXT://:9092 wurstmeister/kafka

# Guarantees no matter what happens, docker will remove the instance at the end.
trap 'remove_containers' EXIT INT TERM

sleep 10
until docker exec -it kafka /bin/bash opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server=localhost:9092
do
  echo 'Kafka is not ready yet'
  sleep 1
done

STORAGE=kafka make storage-integration-test

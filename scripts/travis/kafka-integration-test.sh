#!/bin/bash

set -e

docker pull wurstmeister/kafka
docker pull zookeeper
CID_ZOOKEEPER=$(docker run -d --rm --name zookeeper zookeeper)
CID_KAFKA=$(docker run -d --rm --link zookeeper --name kafka -p 9092:9092 -e KAFKA_ZOOKEEPER_CONNECT=zookeeper -e KAFKA_LISTENERS=PLAINTEXT://:9092 wurstmeister/kafka)

# Guarantees no matter what happens, docker will remove the instance at the end.
trap 'docker rm -f $CID_KAFKA 2>/dev/null' EXIT INT TERM
trap 'docker rm -f $CID_ZOOKEEPER 2>/dev/null' EXIT INT TERM

until docker exec -it kafka /bin/bash opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server=localhost:9092
do
  echo 'Kafka is not ready yet'
  sleep 1
done

STORAGE=kafka make storage-integration-test

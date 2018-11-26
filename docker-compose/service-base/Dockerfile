FROM debian:stretch-slim
RUN apt-get update
RUN apt-get install -y bash ca-certificates netcat python tar wget
RUN wget https://github.com/apache/cassandra/archive/cassandra-3.11.3.tar.gz && \
    tar -xzf cassandra-3.11.3.tar.gz && \
    mv cassandra-cassandra-3.11.3 /cassandra
COPY wait-for-cassandra.sh /bin/

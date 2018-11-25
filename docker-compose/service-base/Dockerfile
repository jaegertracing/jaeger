FROM debian:stretch-slim
RUN apt-get update
RUN apt-get install -y bash ca-certificates netcat python wget
RUN wget http://apache.claz.org/cassandra/3.11.3/apache-cassandra-3.11.3-bin.tar.gz && \
    tar -xzf apache-cassandra-3.11.3-bin.tar.gz && \
    mv apache-cassandra-3.11.3 /cassandra
COPY wait-for-cassandra.sh /bin/

FROM cassandra:3.11

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra
ENTRYPOINT ["/cassandra-schema/docker.sh"]

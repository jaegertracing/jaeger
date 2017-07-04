# TODO: replace this by the final Cassandra image
FROM jpkroehling/cassandra

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra
CMD ["/cassandra-schema/docker.sh"]

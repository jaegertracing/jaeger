# TODO: replace this by the final Cassandra image
FROM jpkroehling/cassandra

COPY cassandra3v001-schema.sh /cassandra-schema/
COPY create-schema.sh /cassandra-schema/

ENV CQLSH_HOST=cassandra
CMD ["/cassandra-schema/create-schema.sh"]

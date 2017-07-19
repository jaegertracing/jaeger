FROM cassandra:3.11

COPY schema/* /cassandra-schema/

ENV CQLSH_HOST=cassandra
CMD ["/cassandra-schema/docker.sh"]

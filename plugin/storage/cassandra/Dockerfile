FROM cassandra:3.11

COPY schema/* /cassandra-schema/

ENTRYPOINT ["/cassandra-schema/docker.sh"]

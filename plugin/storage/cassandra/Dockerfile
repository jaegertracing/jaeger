FROM cassandra:3.11
ENV DATACENTER=dc1 KEYSPACE=jaeger_v1_dc1 MODE=prod
COPY schema/create.sh /
COPY schema/v001.cql.tmpl /
COPY schema/entrypoint-wrapper.sh /
ENTRYPOINT ["/entrypoint-wrapper.sh"]
CMD ["cassandra", "-f"]

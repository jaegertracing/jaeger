FROM cassandra:3.11
ENV DATACENTER=dc1 KEYSPACE=jaeger_v1_dc1 MODE=prod
COPY schema/ /etc/schema
ENTRYPOINT ["/etc/schema/entrypoint-wrapper.sh"]
CMD ["cassandra", "-f"]

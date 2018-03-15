FROM scratch
ENV JAEGER_AGENT_HOST_PORT localhost:6831
EXPOSE 8080 8081 8082 8083
COPY hotrod-linux /go/bin/
CMD /go/bin/hotrod-linux all --jaeger-agent.host-port ${JAEGER_AGENT_HOST_PORT}

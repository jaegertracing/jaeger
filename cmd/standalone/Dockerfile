FROM scratch
# expose agent UDP and sampling ports and query port
EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686

COPY ./jaeger-ui-build /go/src/jaeger-ui-build
COPY ./cmd/standalone/standalone-linux /go/bin/

CMD ["/go/bin/standalone-linux","--span-storage.type=memory","--query.static-files=/go/src/jaeger-ui-build/build/"]

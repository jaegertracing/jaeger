FROM alpine:latest as certs
RUN apk add --update --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 5775/udp 6831/udp 6832/udp 5778
COPY agent-linux /go/bin/
ENTRYPOINT ["/go/bin/agent-linux"]

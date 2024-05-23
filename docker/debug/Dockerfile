FROM golang:1.22.3-alpine AS build
ARG TARGETARCH
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git build-base mailcap
# Remove once go-delve adds support for s390x (https://github.com/go-delve/delve/issues/2883)
RUN if [[ "$TARGETARCH" == "s390x" ]] ; then \
	touch /go/bin/dlv; \
    else \
        go install github.com/go-delve/delve/cmd/dlv@latest; \
    fi

FROM golang:1.22.3-alpine
COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/mime.types /etc/mime.types

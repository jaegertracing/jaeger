FROM node:14 as build-deps
WORKDIR /usr/src/jaeger-ui
COPY jaeger-ui ./
RUN yarn install
RUN yarn build

FROM alpine:3.14 AS cert
RUN apk add --update --no-cache ca-certificates

FROM golang:1.17-alpine3.14
WORKDIR /go/src/jaeger
COPY . ./
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build-deps /usr/src/jaeger-ui/packages/jaeger-ui/build ./jaeger-ui/packages/jaeger-ui/build
RUN apk update && apk add curl \
                          git \
                          protobuf \
                          bash \
                          make \
                          openssh-client && \
     rm -rf /var/cache/apk/* \
RUN apk update && apk add --no-cache gcc musl-dev
RUN export PATH=$PATH:$(go env GOPATH)/bin
RUN make install-tools
RUN make build-query
CMD "/go/src/jaeger/cmd/query/query-linux-amd64"



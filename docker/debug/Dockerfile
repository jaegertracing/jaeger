ARG golang_image

FROM $golang_image AS build
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git && \
    go get github.com/go-delve/delve/cmd/dlv && \
    cd /go/src/github.com/go-delve/delve && \
    make install

FROM $golang_image
COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

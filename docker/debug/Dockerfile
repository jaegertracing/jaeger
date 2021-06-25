ARG golang_image

FROM $golang_image AS build
ARG TARGETARCH
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git
#Once go-delve adds support for s390x (see PR #2948), remove this entire conditional.
RUN if [[ "$TARGETARCH" != "s390x" ]] ; then \
        go get github.com/go-delve/delve/cmd/dlv && \
        cd /go/src/github.com/go-delve/delve && \
        make install; \
    else \
        touch /go/bin/dlv; \
    fi

FROM $golang_image
COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

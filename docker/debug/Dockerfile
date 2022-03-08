ARG golang_image

FROM $golang_image AS build
ARG TARGETARCH
ENV GOPATH /go
RUN apk add --update --no-cache ca-certificates make git build-base mailcap
#Once go-delve adds support for s390x (see PR #2948), remove this entire conditional.
#Once go-delve adds support for ppc64le (see PR go-delve/delve#1564), remove this entire conditional.
RUN if [[ "$TARGETARCH" == "s390x"  ||  "$TARGETARCH" == "ppc64le" ]] ; then \
	touch /go/bin/dlv; \
    else \
        go install github.com/go-delve/delve/cmd/dlv@latest; \
    fi

FROM $golang_image
COPY --from=build /go/bin/dlv /go/bin/dlv
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/mime.types /etc/mime.types

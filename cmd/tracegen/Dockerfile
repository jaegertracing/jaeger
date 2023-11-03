FROM scratch
ARG TARGETARCH
ARG USER_UID=10001

COPY tracegen-linux-$TARGETARCH /go/bin/tracegen-linux
ENTRYPOINT ["/go/bin/tracegen-linux"]
USER ${USER_UID}

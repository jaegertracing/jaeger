ARG base_image
ARG debug_image

FROM $base_image AS release
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY agent-linux-$TARGETARCH /go/bin/agent-linux
EXPOSE 5775/udp 6831/udp 6832/udp 5778/tcp
ENTRYPOINT ["/go/bin/agent-linux"]
USER ${USER_UID}

FROM $debug_image AS debug
ARG TARGETARCH=amd64
ARG USER_UID=10001
COPY agent-debug-linux-$TARGETARCH /go/bin/agent-linux
EXPOSE 5775/udp 6831/udp 6832/udp 5778/tcp 12345/tcp
ENTRYPOINT ["/go/bin/dlv", "exec", "/go/bin/agent-linux", "--headless", "--listen=:12345", "--api-version=2", "--accept-multiclient", "--log", "--"]
USER ${USER_UID}

FROM gliderlabs/alpine

ADD crossdock /
COPY .build/scripts/* .build/scripts/
COPY .build/cmd/* .build/cmd/

EXPOSE 8080
CMD ["/crossdock"]

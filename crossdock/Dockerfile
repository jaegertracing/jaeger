FROM gliderlabs/alpine
ADD crossdock /
CMD ["/crossdock"]
EXPOSE 8080

COPY .build/scripts/* /scripts/
COPY .build/cmd/* /cmd/

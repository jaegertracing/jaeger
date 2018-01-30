# Authorizing proxy collector storage

The collector storage which takes spans and, given a condition, forwards to another collector if condition is met.  

## Enabling

    <collector-command> --span-storage.type=authorizingproxy \
      --authorizingproxy.proxy-host-port 10.0.100.1:14267,... \
      --authorizingproxy.proxy-batch-size 500 \
      --authorizingproxy.proxy-batch-flush-interval-ms 500 \
      --authorizingproxy.proxy-if <authorizing-condition>

### authorizingproxy.proxy-host-port

Comma-delimited list of collector host port strings to which spans should be forwarded.  
No default, use `JAEGER_STORAGE_PROXY_HOST_PORT` environment variable.

### authorizingproxy.proxy-batch-size

Maximum number of elements per source agent to wait for before forwarding to the collectors.  
Default value is `50`, use `JAEGER_STORAGE_PROXY_BATCH_SIZE` environment variable.

### authorizingproxy.proxy-batch-flush-interval-ms

Maximum time to wait for a complete `proxy-batch-size` before forwarding the batches to the collectors. If the `proxy-batch-size` has not been reached, all elements received will be forwarded.  
Default value is `500`, use `JAEGER_STORAGE_PROXY_FLUSH_INTERVAL_MS` environment variable.

### authorizingproxy.proxy-if

Optional condition for the spans to be forwarded. If condition is empty, all spans will be forwarded. Authorizing conditions can be:

    # forward if a given tag matches:
    tag.<tag-name>==string-value

    # forward if a baggage item matches:
    baggage.<key-name>==string-value

No default, use `JAEGER_STORAGE_PROXY_IF` environment variable.
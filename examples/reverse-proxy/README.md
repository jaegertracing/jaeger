# reverse-proxy example

This example illustrates how Jaeger UI can be run behind a reverse proxy under a different URL prefix.

Start the servers:

```sh
cd examples/reverse-proxy
docker compose up
```

Jaeger UI can be accesssed at http://localhost:18080/jaeger/prefix .

# Badger file permissions as non-root service

After the release of 1.50, Jaeger's Docker image is no longer running with root privileges (in [#4783](https://github.com/jaegertracing/jaeger/pull/4783)). In some installations it may cause issues such as "permission denied" errors when writing data.

A possible workaround for this ([proposed here](https://github.com/jaegertracing/jaeger/issues/4906#issuecomment-1991779425)) is to run an initialization step as `root` that pre-creates the Badger data directory and updates its owner to the user that will run the main Jaeger process.

```yaml
version: "3.9"

services:
[...]
  jaeger:
    image: jaegertracing/all-in-one:latest
    command:
      - "--badger.ephemeral=false"
      - "--badger.directory-key=/badger/data/keys"
      - "--badger.directory-value=/badger/data/values"
      - "--badger.span-store-ttl=72h0m0s" # limit storage to 72hrs
    environment:
      - SPAN_STORAGE_TYPE=badger
    # Mount host directory "jaeger_badger_data" as "/badger" inside the container.
    # The actual data directory will be "/badger/data", 
    # since we cannot change permissions on the mount.
    volumes:
      - jaeger_badger_data:/badger
    ports:
      - "16686:16686"
      - "14250"
      - "4317"
    depends_on:
      prepare-data-dir:
        condition: service_completed_successfully

  prepare-data-dir:
    # Run this step as root so that we can change the directory owner.
    user: root
    image: jaegertracing/all-in-one:latest
    command: "/bin/sh -c 'mkdir -p /badger/data && touch /badger/data/.initialized && chown -R 10001:10001 /badger/data'"
    volumes:
      - jaeger_badger_data:/badger

volumes:
  jaeger_badger_data:
```

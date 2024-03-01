`storagereceiver` is a fake receiver that creates an artificial stream of traces by:

- repeatedly querying one of Jaeger storage backends for all traces (by service).
- tracking new traces / spans and passing them to the next component in the pipeline.

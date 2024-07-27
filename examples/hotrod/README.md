# Hot R.O.D. - Rides on Demand

This is a demo application that consists of several microservices and illustrates the use of the OpenTelemetry API & SDK. It can be run standalone, but requires Jaeger backend to view the traces. A tutorial / walkthrough is available:
  * as a blog post [Take Jaeger for a HotROD ride][hotrod-tutorial],
  * as a video [OpenShift Commons Briefing: Distributed Tracing with Jaeger & Prometheus on Kubernetes][hotrod-openshift].

As of Jaeger v1.42.0 this application was upgraded to use the OpenTelemetry SDK for traces.

## Features

* Discover architecture of the whole system via data-driven dependency diagram
* View request timeline & errors, understand how the app works
* Find sources of latency, lack of concurrency
* Highly contextualized logging
* Use baggage propagation to
  * Diagnose inter-request contention (queueing)
  * Attribute time spent in a service
* Use [opentelemetry-go-contrib](https://github.com/open-telemetry/opentelemetry-go-contrib) open source libraries to instrument HTTP and gRPC requests with minimal code changes

## Running

💥💥💥 Try it with Jaeger v2!  See [cmd/jaeger](../../cmd/jaeger).

### Run everything via `docker compose`

* Download `docker-compose.yml` from https://github.com/jaegertracing/jaeger/blob/main/examples/hotrod/docker-compose.yml
* Optional: find the latest Jaeger version (see https://www.jaegertracing.io/download/) and pass it via environment variable `JAEGER_VERSION`. Otherwise `docker compose` will use the `latest` tag, which is fine for the first time you download the images, but once they are in your local registry the `latest` tag is never updated and you may be running stale (and possibly incompatible) verions of Jaeger and the HotROD app.
* Run Jaeger backend and HotROD demo, e.g.:
  * `JAEGER_VERSION=1.52 docker compose -f path-to-yml-file up`
* Access Jaeger UI at http://localhost:16686 and HotROD app at http://localhost:8080
* Shutdown / cleanup with `docker compose -f path-to-yml-file down`

Alternatively, you can run each component separately as described below.

### Run everything in Kubernetes

```bash
kustomize build ./kubernetes | kubectl apply -f -
kubectl port-forward -n example-hotrod service/example-hotrod 8080:frontend
# In another terminal
kubectl port-forward -n example-hotrod service/jaeger 16686:frontend

# To cleanup
kustomize build ./kubernetes | kubectl delete -f -
```

Access Jaeger UI at http://localhost:16686 and HotROD app at http://localhost:8080

### Run Jaeger backend

An all-in-one Jaeger backend is packaged as a Docker container with in-memory storage.

```bash
docker run \
  --rm \
  --name jaeger \
  -p4318:4318 \
  -p16686:16686 \
  -p14268:14268 \
  jaegertracing/all-in-one:latest
```

Jaeger UI can be accessed at http://localhost:16686.

### Run HotROD from source

```bash
git clone git@github.com:jaegertracing/jaeger.git jaeger
cd jaeger
go run ./examples/hotrod/main.go all
```

### Run HotROD from docker
```bash
docker run \
  --rm \
  --link jaeger \
  --env OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318 \
  -p8080-8083:8080-8083 \
  jaegertracing/example-hotrod:latest \
  all
```

Then open http://127.0.0.1:8080

## Metrics

The app exposes metrics in either Go's `expvar` format (by default) or in Prometheus format (enabled via `-m prometheus` flag).
  * `expvar`: `curl http://127.0.0.1:8083/debug/vars`
  * Prometheus: `curl http://127.0.0.1:8083/metrics`

## Linking to traces

The HotROD UI can generate links to the Jaeger UI to find traces corresponding
to each executed request. By default it uses the standard Jaeger UI address
http://localhost:16686, but if your Jaeger UI is running at a different address,
it can be customized via `-j <address>` flag passed to HotROD, e.g.

```
go run ./examples/hotrod/main.go all -j http://jaeger-ui:16686
```

[hotrod-tutorial]: https://medium.com/jaegertracing/take-jaeger-for-a-hotrod-ride-233cf43e46c2
[hotrod-openshift]: https://blog.openshift.com/openshift-commons-briefing-82-distributed-tracing-with-jaeger-prometheus-on-kubernetes/

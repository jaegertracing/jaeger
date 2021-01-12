# Hot R.O.D. - Rides on Demand

This is a demo application that consists of several microservices and illustrates
the use of the OpenTracing API. It can be run standalone, but requires Jaeger backend
to view the traces. A tutorial / walkthrough is available:
  * as a blog post [Take OpenTracing for a HotROD ride][hotrod-tutorial],
  * as a video [OpenShift Commons Briefing: Distributed Tracing with Jaeger & Prometheus on Kubernetes][hotrod-openshift].

## Features

* Discover architecture of the whole system via data-driven dependency diagram
* View request timeline & errors, understand how the app works
* Find sources of latency, lack of concurrency
* Highly contextualized logging
* Use baggage propagation to
  * Diagnose inter-request contention (queueing)
  * Attribute time spent in a service
* Use open source libraries with OpenTracing integration to get vendor-neutral instrumentation for free

## Running

### Run everything via `docker-compose`

* Download `docker-compose.yml` from https://github.com/jaegertracing/jaeger/blob/master/examples/hotrod/docker-compose.yml
* Run Jaeger backend and HotROD demo with `docker-compose -f path-to-yml-file up`
* Access Jaeger UI at http://localhost:16686 and HotROD app at http://localhost:8080
* Shutdown / cleanup with `docker-compose -f path-to-yml-file down`

Alternatively, you can run each component separately as described below.

### Run Jaeger backend

An all-in-one Jaeger backend is packaged as a Docker container with in-memory storage.

```bash
docker run \
  --rm \
  --name jaeger \
  -p6831:6831/udp \
  -p16686:16686 \
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
  --env JAEGER_AGENT_HOST=jaeger \
  --env JAEGER_AGENT_PORT=6831 \
  -p8080-8083:8080-8083 \
  jaegertracing/example-hotrod:latest \
  all
```

Then open http://127.0.0.1:8080

## Metrics

The app exposes metrics in either Go's `expvar` format (by default) or in Prometheus format (enabled via `-m prometheus` flag).
  * `expvar`: `curl http://127.0.0.1:8083/debug/vars`
  * Prometheus: `curl http://127.0.0.1:8083/metrics`

[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
[hotrod-openshift]: https://blog.openshift.com/openshift-commons-briefing-82-distributed-tracing-with-jaeger-prometheus-on-kubernetes/

## Linking to traces

The HotROD UI can generate links to the Jaeger UI to find traces corresponding
to each executed request. By default it uses the standard Jaeger UI address
http://localhost:16686, but if your Jaeger UI is running at a different address,
it can be customized via `-j <address>` flag passed to HotROD, e.g.

```
go run ./examples/hotrod/main.go all -j http://jaeger-ui:16686
```

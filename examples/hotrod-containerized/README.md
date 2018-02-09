# Hot R.O.D. - Rides on Demand - Containerized Version

This is a demo application that consists of several microservices and illustrates
the use of the OpenTracing API. It can be run standalone, but requires Jaeger backend
to view the traces. A tutorial / walkthough is available:
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


## Requirements
- go
- docker-compose
- docker
### Creating the images

```
chmod +x ./create-image.sh
./create-image.sh
```

### Run Everyting
```
docker-compose up
```

Then open http://127.0.0.1:8080


[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
[hotrod-openshift]: https://blog.openshift.com/openshift-commons-briefing-82-distributed-tracing-with-jaeger-prometheus-on-kubernetes

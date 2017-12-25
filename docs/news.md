# Announcing Jaeger v1.0
**2017-12-06**

Today we reached a milestone and released v1.0 of Jaeger backend. Details are in this
[Medium blog post](https://medium.com/jaegertracing/announcing-jaeger-1-0-37b5990cc59b).

# Jaeger Joins Cloud Native Computing Foundation
**2017-09-25**

At the [Open Source Summit NA](http://events.linuxfoundation.org/events/open-source-summit-north-america) in Los Angeles,
the Cloud Native Computing Foundation ([CNCF](http://cncf.io)) announced that it had accepted Jaeger as its 12th hosted project.
Jaeger joins the respected company of other major modern foundational projects like Kubernetes, Prometheus, gRPC, and OpenTracing.

News coverage:

  * [CNCF blog: CNCF Hosts Jaeger](https://www.cncf.io/blog/2017/09/13/cncf-hosts-jaeger/)
  * [Linux Foundation blog: Lyft and Uber on Stage Together at Open Source Summit in L.A.](https://www.linuxfoundation.org/blog/lyft-and-uber-on-stage-together-at-open-source-summit-in-l-a/)
  * [Silicon Angle: Ride-hailing firms Lyft and Uber open-source microservices technology](https://siliconangle.com/blog/2017/09/13/ride-sharing-firms-lyft-uber-donate-microservices-tech-open-source-community/)
  * [The New Stack: CNCF Adds Oracle, Onboards the Envoy and Jaeger Projects](https://thenewstack.io/cncf-adds-oracle-onboards-envoy-jaeger-projects/)
  * [eWeek: Uber and Lyft Bring Open-Source Cloud Projects to CNCF](http://www.eweek.com/cloud/uber-and-lyft-bring-open-source-cloud-projects-to-cncf)
  * [ZDNet: Lyft and Uber travel the same open-source road](http://www.zdnet.com/article/lyft-and-uber-travel-the-same-open-source-road/)

# Introducing Jaeger
**2017-04-14**

<img align="right" src="../images/jaeger-vector.svg" width=400>
Uber is pleased to announce the open source release of Jaeger, a distributed tracing system, used to monitor, profile, and troubleshoot microservices.

Jaeger is written in Go, with OpenTracing compatible client libraries in [Go](https://github.com/jaegertracing/jaeger-client-go), [Java](https://github.com/jaegertracing/jaeger-client-java), [Node](https://github.com/jaegertracing/jaeger-client-node), and [Python](https://github.com/jaegertracing/jaeger-client-python). It allows service owners to instrument their services to get insights into what their architecture is doing.

Jaeger is available now on [Github](https://github.com/jaegertracing/jaeger) as a public beta. Try it out by running the complete backend using the [Docker image](http://jaeger.readthedocs.io/en/latest/getting_started/#all-in-one-docker-image) along with a sample application, [HotROD](http://jaeger.readthedocs.io/en/latest/getting_started/#sample-application), to generate interesting traces.

We hope that other organizations find Jaeger to be a useful tool, and we welcome contributions.
Keep up to date by subscribing to our [mailing list](https://groups.google.com/forum/#!forum/jaeger-tracing).

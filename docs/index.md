#Jaeger Documentation

Welcome to Jaeger's documentation portal! Below, you'll find information for beginners and experienced Jaeger users. 

If you can't find what you are looking for, or have an issue not covered here, we'd love to hear from you either on [Github](https://github.com/uber/jaeger/issues), or on our [Mailing List](https://groups.google.com/forum/#!forum/jaeger-tracing). 

##About
Jaeger, inspired by Google's [Dapper](https://research.google.com/pubs/pub36356.html), is Uber's distributed tracing system which captures timing information on microservice architectures. Currently, this information is used for realtime profiling, and empirically determining service dependencies.

Jaeger is an implementation of the [Opentracing](http://opentracing.io/) standard written in Go; with client libraries available in Go, Java, Node and Python. It uses Cassandra for storage.

##Quick Start
Jaeger provides an all in one image through docker hub to check it out quickly. 

See [Getting Started](getting_started.md#docker) for instructions on how to run this image, and the [Hotrod example](getting_started.md#tracing-a-sample-application) for instructions on creating sample traces. 

##Screenshots
###Traces View
![Traces View](images/traces-ss.png)

###Trace Detail View
![Detail View](images/trace-detail-ss.png)

##History
Read about the evolution of distributed tracing at Uber on the engineering [blog](https://eng.uber.com/distributed-tracing/). 


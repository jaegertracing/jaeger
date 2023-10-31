# Self-assessment

# Self-assessment outline

## Table of contents

* [Metadata](#metadata)
  * [Security links](#security-links)
* [Overview](#overview)
  * [Actors](#actors)
  * [Actions](#actions)
  * [Background](#background)
  * [Goals](#goals)
  * [Non-goals](#non-goals)
* [Self-assessment use](#self-assessment-use)
* [Security functions and features](#security-functions-and-features)
* [Project compliance](#project-compliance)
* [Secure development practices](#secure-development-practices)
* [Security issue resolution](#security-issue-resolution)
* [Appendix](#appendix)

## Metadata

|   |  |
| -- | -- |
| Software | https://github.com/jaegertracing/jaeger/  |
| Security Provider | No  |
| Languages | Go |
| SBOM | [Software bill of materials](https://github.com/jaegertracing/jaeger/releases/latest/download/jaeger-SBOM.spdx.json) |
| | |

### Security links

Provide the list of links to existing security documentation for the project. You may
use the table below as an example:
| Doc | url |
| -- | -- |
| Security file | https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md |

## Overview

Jaeger is an open-source distributed tracing system designed to provide end-to-end visibility into complex distributed architectures. It captures and visualizes traces of requests, allowing developers to monitor and troubleshoot performance issues within their applications.

### Background

Jaeger is an open-source distributed tracing system developed by Uber Technologies and later donated to the Cloud Native Computing Foundation (CNCF). It is designed to help developers monitor and troubleshoot complex, microservices-based architectures by providing insights into the flow of requests and the performance of individual components.

The primary goal of Jaeger is to provide end-to-end visibility into distributed systems. It accomplishes this by capturing and visualizing traces, which are records of the life cycle of a request as it propagates through various services. Traces consist of a sequence of spans, where each span represents a single operation within a service. Spans are connected to form a trace tree, illustrating the causal relationship between different operations.

Key features of Jaeger include:

Trace Collection and Storage: Jaeger provides agents and collectors that capture traces emitted by instrumented services. The collected traces are stored in a back-end storage system, such as Elasticsearch or Apache Cassandra, for further analysis and querying.

Trace Visualization: Jaeger offers a web-based user interface that allows users to explore and analyze traces. It provides features like trace search, filtering, and detailed visualization of spans, enabling developers to identify performance bottlenecks and troubleshoot issues within their applications.

Integration with Ecosystem: Jaeger integrates with various frameworks, libraries, and platforms commonly used in microservices architectures. It provides client libraries for popular programming languages like Java, Go, Python, and more, making it easier to instrument applications for tracing.

Since being donated to the CNCF, Jaeger has gained significant adoption and has become an integral part of the cloud-native ecosystem. It is widely used by organizations to gain insights into the performance and behavior of their distributed systems, aiding in troubleshooting, performance optimization, and overall system understanding. The project continues to evolve and improve with contributions from a vibrant open-source community.
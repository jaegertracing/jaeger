# Hot R.O.D. - Rides on Demand  - Grafana integration

This example combines the Hot R.O.D. demo application ([examples/hotrod/](../hotrod/)) with Grafana, Loki and Prometheus integration, to demonstrate logs, metrics and traces correlation.

## Running via `docker-compose`

### Prerequisites

* Clone the Jaeger repository `git clone https://github.com/jaegertracing/jaeger.git`, then `cd examples/grafana-integration`

* All services will log to Loki via the [official Docker driver plugin](https://grafana.com/docs/loki/latest/clients/docker-driver/).
Install the Loki logging plugin for Docker:

```bash
docker plugin install \
grafana/loki-docker-driver:latest \
--alias loki \
--grant-all-permissions
```

### Run the services

`docker-compose up` 

### Access the services
* HotROD application at http://localhost:8080
* Grafana UI at http://localhost:3000

### Explore with Loki

Currently the most powerful way to correlate application logs with traces can be performed via Grafana's Explore interface.

After setting the datasource to Loki, all the log labels become available, and can be easily filtered using [Loki's LogQL query language](https://grafana.com/docs/loki/latest/logql/).

For example, after selecting the compose project/service under Log labels , errors can be filtered with the following expression:

```
{compose-project="grafana-integration"} |= "error"
```

which will list the redis timeout events.

### HotROD - Metrics and logs overview dashboard

Since the HotROD application can expose its metrics in Prometheus' format, these can be also used during investigation.

This example includes a dashboard that contains a log panel for the selected services in real time. These can be also filtered by a search field, that provides `grep`-like features.

There are also panels to display the ratio/percentage of errors in the current timeframe.

Additionally, there are graphs for each service, visualing the rate of the requests and showing latency percentiles.

### Clean up

`docker-compose down`

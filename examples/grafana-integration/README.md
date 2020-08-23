# Hot R.O.D. - Rides on Demand  - Grafana integration

This is the Hot R.O.D. demo application that consists of the same components as the `examples/hotrod/`, only Grafana and Loki integration is added to this setup, so you can correlate logs and traces in one application.

## Running

### Run everything via `docker-compose`

#### Prerequisites
* Download `docker-compose.yml` from https://github.com/jaegertracing/jaeger/blob/master/examples/grafana-integration/docker-compose.yml
* Download the `datasources.yaml` from the `examples/grafana-integration/` folder
* All services will log to Loki via the loki docker logging plugin.
To install run this on the host once:

```bash
docker plugin install \
grafana/loki-docker-driver:latest \
--alias loki \
--grant-all-permissions
```

#### Deploy the local setup
* Run Grafana and Loki integration with Jaeger backend using HotROD demo with `docker-compose -f path-to-yml-file up`
* Access Grafana UI at http://localhost:3000 and HotROD app at http://localhost:8080
* Shutdown / cleanup with `docker-compose -f path-to-yml-file down`


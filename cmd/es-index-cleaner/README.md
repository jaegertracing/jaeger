# jaeger-es-index-cleaner

It is common to only keep observability data for a limited time.
However, Elasticsearch does not support expiring of old data via TTL.
To help with this task, `jaeger-es-index-cleaner` can be used to purge
old Jaeger indices. For example, to delete indices older than 14 days:

```
docker run -it --rm --net=host -e ROLLOVER=true \
  jaegertracing/jaeger-es-index-cleaner:latest \
  14 \
  http://localhost:9200
```

Another alternative is to use [Elasticsearch Curator][curator].

[curator]: https://www.elastic.co/guide/en/elasticsearch/client/curator/current/about.html

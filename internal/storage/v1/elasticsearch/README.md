# ElasticSearch Support

This provides a storage backend for Jaeger using [Elasticsearch](https://www.elastic.co). More information is available on the [Jaeger documentation website](https://www.jaegertracing.io/docs/latest/deployment/#elasticsearch).

## Authentication

### Basic Authentication
Basic username/password authentication is supported by setting the appropriate flags or environment variables:
```
--es.username=elastic
--es.password=changeme
```

### API Key Authentication
API key authentication is supported as an alternative to basic authentication. This enables seamless key rotation without restarting Jaeger:
```
--es.api-key=<base64-encoded-api-key>
```

You can also load the API key from a file, which will be watched for changes to support rotation:
```
--es.api-key-file=/path/to/api-key-file
```

To generate an API key in Elasticsearch:
```
curl -X POST -u elastic:<elastic-password> -H "Content-Type: application/json" -d '{
  "name": "my-api-key",
  "expiration": "10d",
  "role_descriptors": {
    "my-role": {
      "cluster": ["all"],
      "indices": [
        {
          "names": ["jaeger-*"],
          "privileges": ["read", "write"]
        }
      ]
    }
  }
}' "<elasticsearch-url>/_security/api_key"

```

The response will include the API key in the format:
```json
{
  "id": "VuaCfGcBCdbkQm-e5aOx",
  "name": "jaeger-api-key",
  "expiration": 1623940473549,
  "api_key": "ui2lp2axTNmsyakw9tvNnw"
}
```

You'll need to Base64 encode the `id:api_key` value before using it with Jaeger



## Indices
Indices will be created depending on the spans timestamp. i.e., a span with
a timestamp on 2017/04/21 will be stored in an index named `jaeger-2017-04-21`.

It is common to only keep observability data for a limited time.
However, Elasticsearch does no support expiring of old data via TTL.
To purge old Jaeger indices, use [jaeger-es-index-cleaner](../../../cmd/es-index-cleaner/).

### Timestamps
Because ElasticSearch's `Date` datatype has only millisecond granularity and Jaeger
requires microsecond granularity, Jaeger spans' `StartTime` is saved as a long type.
The conversion is done automatically.

### Nested fields (tags)
`Tags` are [nested](https://www.elastic.co/guide/en/elasticsearch/reference/current/nested.html) fields in the
ElasticSearch schema used for Jaeger. This allows for better search capabilities and data retention. However, because
ElasticSearch creates a new document for every nested field, there is currently a limit of 50 nested fields per document.

### Shards and Replicas
Number of shards and replicas per index can be specified as parameters to the writer and/or through configs under
`./pkg/es/config/config.go`. If not specified, it defaults to ElasticSearch defaults: 5 shards and 1 replica.
[This article](https://www.elastic.co/blog/how-many-shards-should-i-have-in-my-elasticsearch-cluster) goes into more information
about choosing how many shards should be chosen for optimization.

## Limitations

### Tag query over multiple spans
This plugin queries against spans. This means that all tags in a query must lie under the same span for a
query to successfully return a trace.

### Case-sensitivity
Queries are case-sensitive. For example, if a document with service name `ABC` is searched using a query `abc`,
the document will not be retrieved.

## Testing
To locally test the ElasticSearch storage plugin,
* have [ElasticSearch](https://www.elastic.co/guide/en/elasticsearch/reference/current/setup.html) running on port 9200
* run `STORAGE=es make storage-integration-test` in the top folder.

All integration tests also run on pull request via GitHub Actions. This integration test is against ElasticSearch v7.x and v8.x.

* The script used in GitHub Actions can be found under `scripts/e2e/elasticsearch.sh`,
and that script be run from the top folder to integration test ElasticSearch as well.
This script requires Docker to be running.

### Adding tests
Integration test framework for storage lie under `../integration`.
Add to `../integration/fixtures/traces/*.json` and `../integration/fixtures/queries.json` to add more
trace cases.

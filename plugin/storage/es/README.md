# ElasticSearch Support

This provides a plugin to use Jaeger with [ElasticSearch](https://www.elastic.co). This currently supports ElasticSearch 5.x and 6.x.

## Indices
Indices will be created depending on the spans timestamp. i.e., a span with
a timestamp on 2017/04/21 will be stored in an index named `jaeger-2017-04-21`.
ElasticSearch also has no support for TTL, so there exists a script `./esCleaner.py`
that deletes older indices automatically. The [Elastic Curator](https://www.elastic.co/guide/en/elasticsearch/client/curator/current/about.html)
can also be used instead to do a similar job.

### Using `./esCleaner.py`
The script is using `python3`. All dependencies can be installed with: `python3 -m pip install elasticsearch elasticsearch-curator`.

Parameters:
 * Environment variable TIMEOUT that sets the timeout in seconds for indices deletion (default: 120)
 * a number that will delete any indices older than that number in days
 * ElasticSearch hostnames
 * Example usage: `TIMEOUT=120 ./esCleaner.py 4 localhost:9200`

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
[This article](https://qbox.io/blog/optimizing-elasticsearch-how-many-shards-per-index) goes into more information
about choosing how many shards should be chosen for optimization.

## Limitations

### Separation of Spans and Service:Operation Pairs
The current commit has `span` and `service:operation` documents under the same index for a given date.
This is to be separated into two indices in the near future in preparation for ElasticSearch v6.0. (#292)

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

All integration tests also run on pull request via Travis. This integration test is against ElasticSearch v5.4.0.

* The script used in Travis can be found under `./travis/es-integration-test.sh`, 
and that script be run from the top folder to integration test ElasticSearch as well.
This script requires Docker to be running.

### Adding tests
Integration test framework for storage lie under `../integration`. 
Add to `../integration/fixtures/traces/*.json` and `../integration/fixtures/queries.json` to add more
trace cases.

# Badger data storage

## Data modeling

The key design in badger storage backend takes advantage of sorted nature of badger as well as the key only searching with badger, which does not require loading the values from the value log but only the keys in the LSM tree. This is used to implement efficient inverted index for both lookups as well as range searches. All the values in the keys must be stored in big endian ordering to make the sorting work properly.

Index keys structure is created in the ``createIndexKey`` in ``spanstore/writer.go`` and the primary key for spans in ``createTraceKV``.

### Primary key design

Primary keys are the only keys that have a value in the badger's storage. Each key presents a single span, thus a single trace is a collection of tuples. The value is the actual span, which is marshalled into bytes. The marshalling format is indicated by the last 4 bits of the meta encoding byte in the badger entry. 

Primary keys are sorted as follows:

* TraceID High
* TraceID Low
* Timestamp
* SpanID

This allows quick lookup for a single TraceID by searching for prefix with: 0x80 + traceID high + traceID low and then iterating as long as that prefix is valid. Note that timestamp ordering does not allow fetching a range of traces in a time range. 

### Index key design

Each index key has a single byte first to indicate which field is indexed. The last 4 bits of the first byte in the key are used to indicate which index key is used, with the first 4 bits being zeroed. This sorts the LSM tree by index field which allows quicker range queries. Each inverted index key is then sorted in the following order:

* Value
* Timestamp
* TraceID High
* TraceID Low

That means the scanning for a single value can continue until we reach the first timestamp which is not in the boundaries and then stop since we can guarantee the future keys are not going to be valid. 

## Index searches

If the lookup is a single traceID, the logic mentioned in the ``Primary key design`` section is used. If instead we have a TraceQueryParameters with one or more search keys to use, we need to combine the results of multiple index seeks to form an intersection of those results. Each search parameter (each tag is new search parameter) is used to scan single index key, thus we iterate the index until the ``<indexKey><value><timestamp>`` is no longer valid. We do this by checking the prefix for ``<indexKey><value>`` for exactness and then ``<timestamp>`` for range. As long as that one is valid, we fetch the keys. Once the timestamp goes beyond our maximum timestamp, the iteration stops. The keys are then sorted to ``TraceID`` order instead of their natural key ordering for the next part.

Exception to the above is the duration index, since there are no exact duration values but a range of values. When scanning it, the prefix search lookups the starting point with ``<indexKey><minDurationValue>`` and scans the index until ``<indexKey><maxDurationValue>`` is reached. Each key is then separately checked for valid ``<timestamp>`` but the timestamp does not control the seek process and some keys are ignored because they did not match the given time range. 

Because each TraceID is stored as spans, the same TraceID can appear multiple times from a index query. Other than duration query, this means they are coming in order so each of them is discarded by easily checking if the previous one is equal to current one, but with the duration index the spans can come in random order and thus hash-join is used to filter the duplicates.

After all the index keys have been scanned, the process is then sent to the merge-join where two index queries are compared and only matching IDs are taken. After that, the next one is compared to the result of the previous and so forth until all the index fetches have been processed. The resulting query set is the list of TraceIDs that matched all the requirements. 
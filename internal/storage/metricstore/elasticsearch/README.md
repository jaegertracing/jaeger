## `getCallRate` Calculation Explained

The `getCallRate` method calculates the call rate (requests per second) for a service by querying span data stored in Elasticsearch. The process involves three key stages: filtering the relevant spans, performing a time-series aggregation to count requests, and post-processing the aggregated data to calculate the final rate.

This document breaks down each of these stages, referencing the corresponding parts of the Elasticsearch query and the Go implementation.

### 1\. Filter Query Part

The first step is to isolate the specific set of documents (spans) needed for the calculation. We use a `bool` query with a `filter` clause, which is efficient as it doesn't contribute to document scoring.

**ES Query Reference:**

```json
"query": {
  "bool": {
    "filter": [
      { "term": { "process.serviceName": "${service}" } },
      {
        "nested": {
          "path": "tags",
          "query": {
            "bool": {
              "must": [
                { "term": { "tags.key": "span.kind" } },
                { "term": { "tags.value": "server" } }
              ]
            }
          }
        }
      },
      {
        "range": {
          "startTimeMillis": {
            "gte": "now-6h",
            "lte": "now",
            "format": "epoch_millis"
          }
        }
      }
    ]
  }
}
```

**Explanation:**

* **`{ "term": { "process.serviceName": "${service}" } }`**: This filter selects spans that belong to the specified service. This is the primary entity for which we are calculating the call rate.
* **`{ "nested": { ... "tags.value": "server" } }`**: This is a critical filter for correctly calculating the *incoming* call rate. By filtering for spans where `span.kind` is `server`, we ensure that we are only counting spans that represent a server receiving a request. This prevents us from incorrectly counting outgoing calls made by the service.
* **`{ "range": { "startTimeMillis": ... } }`**: This filter restricts the spans to a specific time window. The `getCallRate` implementation uses an extended time range (by adding a 10-minute lookback period via `extendedStartTimeMillis`). This is done to ensure that when we calculate the rate for the earliest time points in our requested window, we have sufficient historical data to compute a meaningful value.

**Code Reference:**

This logic is constructed in the `buildQuery` method. The filters are progressively added to a `boolQuery`.

-----

### 2\. Aggregation Query Part

After filtering the spans, we need to aggregate them into a time series that we can use to calculate a rate. The query does not calculate the rate directly; instead, it prepares the data by creating a running total of requests over time.

Note: The reference ES query in the prompt includes a `moving_fn` aggregation to calculate the rate within Elasticsearch. However, the `getCallRate` method in the provided Go code uses a different approach: it fetches the cumulative sum and calculates the rate in the application layer, as described in the Post-Processing section. The aggregation below reflects the logic in the Go code.

**ES Query Reference (as implemented in `buildCallRateAggregations`):**

```json
"aggs": {
  "requests_per_bucket": {
    "date_histogram": {
      "field": "startTimeMillis",
      "fixed_interval": "60s",
      "min_doc_count": 0,
      "extended_bounds": {
        "min": "now-6h",
        "max": "now"
      }
    },
    "aggs": {
      "cumulative_requests": {
        "cumulative_sum": { "buckets_path": "_count" }
      }
    }
  }
}
```

**Explanation:**

1.  **`date_histogram`**: This aggregation is the foundation of our time series. It groups the filtered spans into time buckets of a `fixed_interval` (e.g., `60s`). For each bucket, it provides a count (`_count`) of the documents (i.e., server spans) that fall within that time interval.

2.  **`cumulative_sum`**: This is a sub-aggregation that operates on the buckets created by the `date_histogram`. It calculates a running total of the document counts. For any given time bucket, its `cumulative_requests` value is the sum of all `_count`s from the very first bucket up to and including the current one.

**Code Reference:**

This aggregation pipeline is constructed in the `buildCallRateAggregations` method.

-----

### 3\. Post-Processing Part

The final step happens in the application layer, within the `getCallRateProcessMetrics` function. This function takes the time series of `(timestamp, cumulative_request_count)` pairs returned by Elasticsearch and transforms it into a series of call rates.

**Explanation:**

The function implements a sliding window algorithm to calculate the rate. It iterates through each data point and, for each point, it calculates the average rate over a preceding "lookback" period.

The core calculation for each point in the time series is:

$$\text{rate} = \frac{\Delta \text{Value}}{\Delta \text{Time}} = \frac{\text{lastVal} - \text{firstVal}}{\text{windowSizeSeconds}}$$

Where:

* `lastVal`: The cumulative request count at the end of the sliding window (the current data point).
* `firstVal`: The cumulative request count at the beginning of the sliding window.
* `lastVal - firstVal`: The total number of new requests that occurred during the window.
* `windowSizeSeconds`: The duration of the sliding window in seconds.

**Why this approach?**

This post-processing logic effectively calculates the slope of the cumulative requests graph over a sliding window, which is the definition of a rate. Performing this calculation client-side provides several advantages:

* **Flexibility:** It gives full control over the rate calculation logic and how to handle edge cases, such as intervals with no data (`NaN` values).
* **Simplicity:** It keeps the Elasticsearch query relatively simple and offloads potentially complex scripting from the database, which can be more performant and easier to maintain.
* **Clarity:** The logic is explicitly defined in the Go code, making it clear how the final metric is derived from the raw cumulative counts.

**Code Reference:**

The post-processing logic resides in `getCallRateProcessMetrics`, which is passed as a function pointer to the main query executor in `GetCallRates`.
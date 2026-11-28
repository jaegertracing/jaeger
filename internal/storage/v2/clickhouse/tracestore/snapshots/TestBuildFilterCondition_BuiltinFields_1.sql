-- eq on the trace ID
-- binds [00000000000000000000000000000f01]
s.trace_id = ?
-- eq on the span ID
-- binds [0000000000000f01]
s.id = ?
-- exists on the parent span ID, which a root span lacks
-- binds []
s.parent_span_id != ''
-- eq on the trace state
-- binds [congo=t61rcWkgMzE]
(s.trace_state != '' AND s.trace_state = ?)
-- ne on the status message, which a span can lack
-- binds [timeout]
(s.status_message != '' AND NOT (s.status_message = ?))
-- lt on the status message, which a span can lack
-- binds [timeout]
(s.status_message != '' AND s.status_message < ?)
-- gt on the span name orders text
-- binds [m]
s.name > ?
-- gt on the start time
-- binds [1767323045000000006]
s.start_time > fromUnixTimestamp64Nano(?)
-- lte on the end time, which is computed
-- binds [1767323045000000006]
addNanoseconds(s.start_time, s.duration) <= fromUnixTimestamp64Nano(?)
-- in on the start time
-- binds [1767323045000000006 1767323046000000006]
s.start_time IN (fromUnixTimestamp64Nano(?), fromUnixTimestamp64Nano(?))
-- eq on the scope name
-- binds [worker-scope]
(s.scope_name != '' AND s.scope_name = ?)
-- not_in on the scope version, which a span can lack
-- binds [1.0 2.0]
(s.scope_version != '' AND NOT (s.scope_version IN (?, ?)))
-- eq on the name of an event
-- binds [exception]
arrayExists(x -> x.name = ?, s.events)
-- ne on the name of an event
-- binds [exception]
(arrayExists(x -> true, s.events) AND NOT arrayExists(x -> x.name = ?, s.events))
-- exists on the name of an event
-- binds []
arrayExists(x -> true, s.events)
-- in on the name of an event
-- binds [exception retry]
arrayExists(x -> x.name IN (?, ?), s.events)
-- not_in on the name of an event
-- binds [exception retry]
(arrayExists(x -> true, s.events) AND NOT arrayExists(x -> x.name IN (?, ?), s.events))
-- gt on the time of an event
-- binds [1767323045000000006]
arrayExists(x -> x.timestamp > fromUnixTimestamp64Nano(?), s.events)
-- gt on the time of an event since the start of its span, which is computed
-- binds [50000]
arrayExists(x -> (toUnixTimestamp64Nano(x.timestamp) - toUnixTimestamp64Nano(s.start_time)) > ?, s.events)
-- eq on the trace ID of a link
-- binds [00000000000000000000000000000f01]
arrayExists(x -> x.trace_id = ?, s.links)
-- eq on the span ID of a link
-- binds [0000000000000f01]
arrayExists(x -> x.span_id = ?, s.links)
-- exists on the trace state of a link, which a link can lack
-- binds []
arrayExists(x -> x.trace_state != '', s.links)
-- ne on the trace state of a link
-- binds [congo=t61rcWkgMzE]
(arrayExists(x -> x.trace_state != '', s.links) AND NOT arrayExists(x -> x.trace_state != '' AND x.trace_state = ?, s.links))
-- lt on the trace state of a link
-- binds [congo=u]
arrayExists(x -> x.trace_state != '' AND x.trace_state < ?, s.links)
-- in on the trace state of a link
-- binds [congo=t61rcWkgMzE]
arrayExists(x -> x.trace_state != '' AND x.trace_state IN (?), s.links)

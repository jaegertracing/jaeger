-- exists on a span attribute that the metadata does not know is false
-- binds []
(
	false
)
-- exists on an unqualified attribute that the metadata reports as a string on the span and on the resource
-- binds [zone zone]
(
	(has(s.str_attributes.key, ?))
	OR
	(has(s.resource_str_attributes.key, ?))
)
-- exists on an event attribute that the metadata reports as a string
-- binds [zone]
(
	arrayExists(x -> has(x.str_attributes.key, ?), s.events)
)
-- exists on a span attribute that the metadata reports as an int and as a string
-- binds [http.status_code http.status_code]
(
	(has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?))
)
-- exists on an unqualified attribute that the metadata reports on the span only
-- binds [http.status_code http.status_code]
(
	(has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?))
)
-- exists on a resource attribute that the metadata reports on the span only is false
-- binds []
(
	false
)
-- exists on a span attribute that the metadata reports as bytes and as a map
-- binds [@bytes@payload @map@payload]
(
	(has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
)
-- ne on a resource attribute that the metadata reports on the span only is false
-- binds []
(
	(
		false
	)
	AND NOT
	(
		false
	)
)
-- exists on a field that every span has
-- binds []
true
-- exists on a field that a span can lack
-- binds []
s.service_name != ''
-- ne on a span attribute with a string constant
-- binds [zone zone us-east]
(
	(
		(has(s.str_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	)
)
-- ne on an unqualified attribute that the metadata does not know is false
-- binds []
(
	(
		false
	)
	AND NOT
	(
		false
	)
)
-- ne on an unqualified attribute with an untyped constant, for an attribute that the metadata reports as a string on the span and on the resource
-- binds [zone zone zone us-east zone us-east]
(
	(
		(has(s.str_attributes.key, ?))
		OR
		(has(s.resource_str_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
	)
)
-- ne on a span attribute that the metadata reports as an int and as a string
-- binds [http.status_code http.status_code http.status_code 200 http.status_code 200]
(
	(
		(has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	)
)
-- ne on a link attribute
-- binds [zone zone us-east]
(
	(
		arrayExists(x -> has(x.str_attributes.key, ?), s.links)
	)
	AND NOT
	(
		arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links)
	)
)
-- ne on a field that every span has
-- binds [Error]
s.status_code != ?
-- ne on a field that a span can lack
-- binds [cart]
(s.service_name != '' AND s.service_name != ?)
-- in on a span attribute with an untyped list, for an attribute that the metadata reports as an int and as a string
-- binds [http.status_code 200 http.status_code 200 http.status_code 5xx]
(
	arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
)
-- in on a resource attribute with a list of ints
-- binds [shard 1 shard 2]
(
	arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
)
-- in on a string field
-- binds [GET /cart POST /cart]
s.name IN (?, ?)
-- in on the span kind
-- binds [server ]
s.kind IN (?, ?)
-- in on the duration
-- binds [1000000000 2000000000]
s.duration IN (?, ?)
-- in on a field that a span can lack, with an empty element
-- binds []
(s.service_name != '' AND s.service_name IN (?))
-- not_in on an unqualified attribute
-- binds [zone zone zone us-east zone us-east zone eu-west zone eu-west]
(
	(
		(has(s.str_attributes.key, ?))
		OR
		(has(s.resource_str_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
	)
)
-- not_in on a field that every span has
-- binds [GET /cart]
s.name NOT IN (?)
-- not_in on a field that a span can lack
-- binds [cart checkout]
(s.service_name != '' AND s.service_name NOT IN (?, ?))

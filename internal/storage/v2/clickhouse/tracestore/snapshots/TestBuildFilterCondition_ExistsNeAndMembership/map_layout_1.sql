-- exists on a span attribute that the metadata does not know is false
-- binds []
(
	false
)
-- exists on an unqualified attribute that the metadata reports as a string on the span and on the resource
-- binds [zone zone]
(
	(mapContains(s.str_attributes, ?))
	OR
	(mapContains(s.resource_str_attributes, ?))
)
-- exists on an event attribute that the metadata reports as a string
-- binds [zone]
(
	arrayExists(x -> mapContains(x.str_attributes, ?), s.events)
)
-- exists on a span attribute that the metadata reports as an int and as a string
-- binds [http.status_code http.status_code]
(
	(mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?))
)
-- exists on an unqualified attribute that the metadata reports on the span only
-- binds [http.status_code http.status_code]
(
	(mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?))
)
-- exists on a resource attribute that the metadata reports on the span only is false
-- binds []
(
	false
)
-- exists on a span attribute that the metadata reports as bytes and as a map
-- binds [@bytes@payload @map@payload]
(
	(mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
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
-- binds [zone zone zone us-east]
(
	(
		(mapContains(s.str_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
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
-- binds [zone zone zone zone us-east zone zone us-east]
(
	(
		(mapContains(s.str_attributes, ?))
		OR
		(mapContains(s.resource_str_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
		OR
		(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?)
	)
)
-- ne on a span attribute that the metadata reports as an int and as a string
-- binds [http.status_code http.status_code http.status_code http.status_code 200 http.status_code http.status_code 200]
(
	(
		(mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?)
		OR
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	)
)
-- ne on a link attribute
-- binds [zone zone zone us-east]
(
	(
		arrayExists(x -> mapContains(x.str_attributes, ?), s.links)
	)
	AND NOT
	(
		arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links)
	)
)
-- ne on a field that every span has
-- binds [Error]
NOT (s.status_code = ?)
-- ne on a field that a span can lack
-- binds [cart]
(s.service_name != '' AND NOT (s.service_name = ?))
-- in on a span attribute with an untyped list, for an attribute that the metadata reports as an int and as a string
-- binds [http.status_code http.status_code 200 http.status_code http.status_code 200 http.status_code http.status_code 5xx]
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
)
-- in on a resource attribute with a list of ints
-- binds [shard shard 1 shard shard 2]
(
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?)
	OR
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?)
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
-- binds [zone zone zone zone us-east zone zone us-east zone zone eu-west zone zone eu-west]
(
	(
		(mapContains(s.str_attributes, ?))
		OR
		(mapContains(s.resource_str_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
		OR
		(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?)
		OR
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
		OR
		(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?)
	)
)
-- not_in on a field that every span has
-- binds [GET /cart]
NOT (s.name IN (?))
-- not_in on a field that a span can lack
-- binds [cart checkout]
(s.service_name != '' AND NOT (s.service_name IN (?, ?)))

-- exists on a span attribute
(
	(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone]
-- exists on an unqualified attribute searches the span and the resource
(
	(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
	OR
	(has(s.resource_bool_attributes.key, ?) OR has(s.resource_double_attributes.key, ?) OR has(s.resource_int_attributes.key, ?) OR has(s.resource_str_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?))
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone]
-- exists on an event attribute
(
	arrayExists(x -> has(x.bool_attributes.key, ?) OR has(x.double_attributes.key, ?) OR has(x.int_attributes.key, ?) OR has(x.str_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?), s.events)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone]
-- exists on a field that every span has
true
-- binds []
-- exists on a field that a span can lack
s.service_name != ''
-- binds []
-- ne on a span attribute with a string constant
(
	(
		(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone us-east]
-- ne on an unqualified attribute with an untyped constant searches every type that the constant reads as
(
	(
		(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
		OR
		(has(s.resource_bool_attributes.key, ?) OR has(s.resource_double_attributes.key, ?) OR has(s.resource_int_attributes.key, ?) OR has(s.resource_str_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
	)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone us-east zone us-east]
-- ne on a span attribute with an untyped constant that reads as a number
(
	(
		(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
	)
	AND NOT
	(
		arrayExists((key, value) -> key = ? AND value = ?, s.double_attributes.key, s.double_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
		OR
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	)
)
-- binds [http.status_code http.status_code http.status_code http.status_code @bytes@http.status_code @map@http.status_code @slice@http.status_code http.status_code 200 http.status_code 200 http.status_code 200]
-- ne on a link attribute
(
	(
		arrayExists(x -> has(x.bool_attributes.key, ?) OR has(x.double_attributes.key, ?) OR has(x.int_attributes.key, ?) OR has(x.str_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?) OR has(x.complex_attributes.key, ?), s.links)
	)
	AND NOT
	(
		arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.links)
	)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone us-east]
-- ne on a field that every span has
s.status_code != ?
-- binds [Error]
-- ne on a field that a span can lack
(s.service_name != '' AND s.service_name != ?)
-- binds [cart]
-- in on a span attribute with an untyped list, for an attribute that the metadata reports as an int and as a string
(
	arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
)
-- binds [http.status_code 200 http.status_code 200 http.status_code 5xx]
-- in on a resource attribute with a list of ints
(
	arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
)
-- binds [shard 1 shard 2]
-- in on a string field
s.name IN (?, ?)
-- binds [GET /cart POST /cart]
-- in on the span kind
s.kind IN (?, ?)
-- binds [server ]
-- in on the duration
s.duration IN (?, ?)
-- binds [1000000000 2000000000]
-- in on a field that a span can lack, with an empty element
(s.service_name != '' AND s.service_name IN (?))
-- binds []
-- not_in on an unqualified attribute
(
	(
		(has(s.bool_attributes.key, ?) OR has(s.double_attributes.key, ?) OR has(s.int_attributes.key, ?) OR has(s.str_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?) OR has(s.complex_attributes.key, ?))
		OR
		(has(s.resource_bool_attributes.key, ?) OR has(s.resource_double_attributes.key, ?) OR has(s.resource_int_attributes.key, ?) OR has(s.resource_str_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?) OR has(s.resource_complex_attributes.key, ?))
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
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone us-east zone us-east zone eu-west zone eu-west]
-- not_in on a field that every span has
s.name NOT IN (?)
-- binds [GET /cart]
-- not_in on a field that a span can lack
(s.service_name != '' AND s.service_name NOT IN (?, ?))
-- binds [cart checkout]

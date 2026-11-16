-- exists on a span attribute
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone]
(
	(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
)
-- exists on an unqualified attribute searches the span and the resource
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone]
(
	(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
	OR
	(mapContains(s.resource_bool_attributes, ?) OR mapContains(s.resource_double_attributes, ?) OR mapContains(s.resource_int_attributes, ?) OR mapContains(s.resource_str_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?))
)
-- exists on an event attribute
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone]
(
	arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.events)
)
-- exists on a field that every span has
-- binds []
true
-- exists on a field that a span can lack
-- binds []
s.service_name != ''
-- ne on a span attribute with a string constant
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east]
(
	(
		(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	)
)
-- ne on an unqualified attribute with an untyped constant searches every type that the constant reads as
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east zone zone us-east]
(
	(
		(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
		OR
		(mapContains(s.resource_bool_attributes, ?) OR mapContains(s.resource_double_attributes, ?) OR mapContains(s.resource_int_attributes, ?) OR mapContains(s.resource_str_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
		OR
		(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] = ?)
	)
)
-- ne on a span attribute with an untyped constant that reads as a number
-- binds [http.status_code http.status_code http.status_code http.status_code @bytes@http.status_code @map@http.status_code @slice@http.status_code http.status_code http.status_code 200 http.status_code http.status_code 200 http.status_code http.status_code 200]
(
	(
		(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.double_attributes, ?) AND s.double_attributes[?] = ?)
		OR
		(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?)
		OR
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	)
)
-- ne on a link attribute
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east]
(
	(
		arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.links)
	)
	AND NOT
	(
		arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links)
	)
)
-- ne on a field that every span has
-- binds [Error]
s.status_code != ?
-- ne on a field that a span can lack
-- binds [cart]
(s.service_name != '' AND s.service_name != ?)
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
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east zone zone us-east zone zone eu-west zone zone eu-west]
(
	(
		(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
		OR
		(mapContains(s.resource_bool_attributes, ?) OR mapContains(s.resource_double_attributes, ?) OR mapContains(s.resource_int_attributes, ?) OR mapContains(s.resource_str_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?))
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
s.name NOT IN (?)
-- not_in on a field that a span can lack
-- binds [cart checkout]
(s.service_name != '' AND s.service_name NOT IN (?, ?))

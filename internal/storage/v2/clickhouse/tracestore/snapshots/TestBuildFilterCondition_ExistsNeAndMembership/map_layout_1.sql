-- exists on a span attribute
(
	(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone]
-- exists on an unqualified attribute searches the span and the resource
(
	(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
	OR
	(mapContains(s.resource_bool_attributes, ?) OR mapContains(s.resource_double_attributes, ?) OR mapContains(s.resource_int_attributes, ?) OR mapContains(s.resource_str_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?) OR mapContains(s.resource_complex_attributes, ?))
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone]
-- exists on an event attribute
(
	arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.events)
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
		(mapContains(s.bool_attributes, ?) OR mapContains(s.double_attributes, ?) OR mapContains(s.int_attributes, ?) OR mapContains(s.str_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?) OR mapContains(s.complex_attributes, ?))
	)
	AND NOT
	(
		(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east]
-- ne on an unqualified attribute with an untyped constant searches every type that the constant reads as
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
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east zone zone us-east]
-- ne on a span attribute with an untyped constant that reads as a number
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
-- binds [http.status_code http.status_code http.status_code http.status_code @bytes@http.status_code @map@http.status_code @slice@http.status_code http.status_code http.status_code 200 http.status_code http.status_code 200 http.status_code http.status_code 200]
-- ne on a link attribute
(
	(
		arrayExists(x -> mapContains(x.bool_attributes, ?) OR mapContains(x.double_attributes, ?) OR mapContains(x.int_attributes, ?) OR mapContains(x.str_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?) OR mapContains(x.complex_attributes, ?), s.links)
	)
	AND NOT
	(
		arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] = ?, s.links)
	)
)
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east]
-- ne on a field that every span has
s.status_code != ?
-- binds [Error]
-- ne on a field that a span can lack
(s.service_name != '' AND s.service_name != ?)
-- binds [cart]
-- in on a span attribute with an untyped list, for an attribute that the metadata reports as an int and as a string
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] = ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] = ?)
)
-- binds [http.status_code http.status_code 200 http.status_code http.status_code 200 http.status_code http.status_code 5xx]
-- in on a resource attribute with a list of ints
(
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?)
	OR
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] = ?)
)
-- binds [shard shard 1 shard shard 2]
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
-- binds [zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone zone zone @bytes@zone @map@zone @slice@zone zone zone us-east zone zone us-east zone zone eu-west zone zone eu-west]
-- not_in on a field that every span has
s.name NOT IN (?)
-- binds [GET /cart]
-- not_in on a field that a span can lack
(s.service_name != '' AND s.service_name NOT IN (?, ?))
-- binds [cart checkout]

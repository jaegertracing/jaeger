-- an int constant orders the int values
-- binds [retry.count retry.count 10]
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] > ?)
)
-- a double constant orders the double values
-- binds [load load 0.5]
(
	(mapContains(s.double_attributes, ?) AND s.double_attributes[?] <= ?)
)
-- a string constant orders the string values as text
-- binds [zone zone m]
(
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] < ?)
)
-- an untyped constant orders the values of the types that the metadata reports, an int and a string
-- binds [http.status_code http.status_code 500 http.status_code http.status_code 500]
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] >= ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] >= ?)
)
-- an untyped constant with a fraction is still compared with the int values
-- binds [http.status_code http.status_code 499.5 http.status_code http.status_code 499.5]
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] > ?)
	OR
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] > ?)
)
-- an untyped constant that is not a number is compared with the string values only
-- binds [http.status_code http.status_code 5xx]
(
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] < ?)
)
-- an untyped constant on a resource attribute that the metadata reports on the span only is false
-- binds []
(
	false
)
-- an untyped constant on an attribute that the metadata does not know is false
-- binds []
(
	false
)
-- an unqualified attribute searches the span and the resource
-- binds [retry.count retry.count 10 retry.count retry.count 10]
(
	(mapContains(s.int_attributes, ?) AND s.int_attributes[?] > ?)
	OR
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] > ?)
)
-- an attribute of the resource
-- binds [shard shard 8]
(
	(mapContains(s.resource_int_attributes, ?) AND s.resource_int_attributes[?] < ?)
)
-- an attribute of the scope
-- binds [build build 3]
(
	(mapContains(s.scope_int_attributes, ?) AND s.scope_int_attributes[?] >= ?)
)
-- an attribute of an event
-- binds [attempt attempt 1]
(
	arrayExists(x -> mapContains(x.int_attributes, ?) AND x.int_attributes[?] > ?, s.events)
)
-- an attribute of a link
-- binds [weight weight 0.5]
(
	arrayExists(x -> mapContains(x.double_attributes, ?) AND x.double_attributes[?] <= ?, s.links)
)

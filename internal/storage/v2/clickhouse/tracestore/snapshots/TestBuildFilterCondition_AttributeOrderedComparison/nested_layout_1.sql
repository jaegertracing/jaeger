-- an int constant orders the int values
-- binds [retry.count 10]
(
	arrayExists((key, value) -> key = ? AND value > ?, s.int_attributes.key, s.int_attributes.value)
)
-- a double constant orders the double values
-- binds [load 0.5]
(
	arrayExists((key, value) -> key = ? AND value <= ?, s.double_attributes.key, s.double_attributes.value)
)
-- a string constant orders the string values as text
-- binds [zone m]
(
	arrayExists((key, value) -> key = ? AND value < ?, s.str_attributes.key, s.str_attributes.value)
)
-- an untyped constant orders the values of the types that the metadata reports, an int and a string
-- binds [http.status_code 500 http.status_code 500]
(
	arrayExists((key, value) -> key = ? AND value >= ?, s.int_attributes.key, s.int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value >= ?, s.str_attributes.key, s.str_attributes.value)
)
-- an untyped constant with a fraction is still compared with the int values
-- binds [http.status_code 499.5 http.status_code 499.5]
(
	arrayExists((key, value) -> key = ? AND value > ?, s.int_attributes.key, s.int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value > ?, s.str_attributes.key, s.str_attributes.value)
)
-- an untyped constant that is not a number is compared with the string values only
-- binds [http.status_code 5xx]
(
	arrayExists((key, value) -> key = ? AND value < ?, s.str_attributes.key, s.str_attributes.value)
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
-- binds [retry.count 10 retry.count 10]
(
	arrayExists((key, value) -> key = ? AND value > ?, s.int_attributes.key, s.int_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND value > ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
)
-- an attribute of the resource
-- binds [shard 8]
(
	arrayExists((key, value) -> key = ? AND value < ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
)
-- an attribute of the scope
-- binds [build 3]
(
	arrayExists((key, value) -> key = ? AND value >= ?, s.scope_int_attributes.key, s.scope_int_attributes.value)
)
-- an attribute of an event
-- binds [attempt 1]
(
	arrayExists(x -> arrayExists((key, value) -> key = ? AND value > ?, x.int_attributes.key, x.int_attributes.value), s.events)
)
-- an attribute of a link
-- binds [weight 0.5]
(
	arrayExists(x -> arrayExists((key, value) -> key = ? AND value <= ?, x.double_attributes.key, x.double_attributes.value), s.links)
)

-- a pattern on the span name
-- binds [^GET /cart]
s.name REGEXP ?
-- a pattern with a dot clears the flag that lets the dot match a newline
-- binds [(?-s)^GET /c.rt|^POST]
s.name REGEXP ?
-- a pattern whose dot matches a newline is sent as written
-- binds [(?s)^GET.*cart]
s.name REGEXP ?
-- a pattern that is a typed string
-- binds [^GET /cart]
s.name REGEXP ?
-- a pattern on a field that a span can lack
-- binds [(?-s)time.?out]
(s.status_message != '' AND s.status_message REGEXP ?)
-- a pattern on the span kind matches the word, not the stored value
-- binds [^(server|unspecified)$]
if(s.kind = '', 'unspecified', s.kind) REGEXP ?
-- a pattern on the span status matches the word, not the stored value
-- binds [^(error|unset)$]
lower(s.status_code) REGEXP ?
-- a pattern on the name of an event
-- binds [^cache]
arrayExists(x -> x.name REGEXP ?, s.events)
-- a pattern on the trace state of a link, which a link can lack
-- binds [^congo=]
arrayExists(x -> x.trace_state != '' AND x.trace_state REGEXP ?, s.links)
-- a pattern on a span attribute
-- binds [http.route http.route ^/api/]
(
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] REGEXP ?)
)
-- a pattern on an unqualified attribute searches the span and the resource
-- binds [zone zone ^us- zone zone ^us-]
(
	(mapContains(s.str_attributes, ?) AND s.str_attributes[?] REGEXP ?)
	OR
	(mapContains(s.resource_str_attributes, ?) AND s.resource_str_attributes[?] REGEXP ?)
)
-- a pattern on a scope attribute
-- binds [library.language library.language ^go]
(
	(mapContains(s.scope_str_attributes, ?) AND s.scope_str_attributes[?] REGEXP ?)
)
-- a pattern on a link attribute
-- binds [zone zone ^us-]
(
	arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] REGEXP ?, s.links)
)
-- a pattern on an event attribute
-- binds [exception.type exception.type Error$]
(
	arrayExists(x -> mapContains(x.str_attributes, ?) AND x.str_attributes[?] REGEXP ?, s.events)
)

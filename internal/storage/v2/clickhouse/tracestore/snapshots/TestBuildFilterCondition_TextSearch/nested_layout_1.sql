-- every listed word in a span attribute, with ASCII letters lowercased
-- binds [note [premium refund Über]]
(
	arrayExists((key, value) -> key = ? AND hasAllTokens(lower(value), ?), s.str_attributes.key, s.str_attributes.value)
)
-- the listed words adjacent and in order in a span attribute, with ASCII letters lowercased
-- binds [note [refund policy]]
(
	arrayExists((key, value) -> key = ? AND hasSubstr(tokens(lower(value)), ?), s.str_attributes.key, s.str_attributes.value)
)
-- words in an unqualified attribute search the span and the resource
-- binds [note [refund] note [refund]]
(
	arrayExists((key, value) -> key = ? AND hasAllTokens(lower(value), ?), s.str_attributes.key, s.str_attributes.value)
	OR
	arrayExists((key, value) -> key = ? AND hasAllTokens(lower(value), ?), s.resource_str_attributes.key, s.resource_str_attributes.value)
)
-- a phrase in an event attribute
-- binds [exception.message [connection refused]]
(
	arrayExists(x -> arrayExists((key, value) -> key = ? AND hasSubstr(tokens(lower(value)), ?), x.str_attributes.key, x.str_attributes.value), s.events)
)
-- words in a link attribute
-- binds [note [refund]]
(
	arrayExists(x -> arrayExists((key, value) -> key = ? AND hasAllTokens(lower(value), ?), x.str_attributes.key, x.str_attributes.value), s.links)
)
-- a phrase in an attribute of the event that a some operator binds
-- binds [exception exception.message [connection refused]]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(
			arrayExists((key, value) -> key = ? AND hasSubstr(tokens(lower(value)), ?), x.str_attributes.key, x.str_attributes.value)
		)
	)
), s.events)
-- a negated phrase also matches a span without the attribute
-- binds [note [refund policy]]
NOT (
	(
		arrayExists((key, value) -> key = ? AND hasSubstr(tokens(lower(value)), ?), s.str_attributes.key, s.str_attributes.value)
	)
)

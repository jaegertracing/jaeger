-- two fields of one event
-- binds [exception 50000]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(toUnixTimestamp64Nano(x.timestamp) - toUnixTimestamp64Nano(s.start_time)) > ?
	)
), s.events)
-- a field and an attribute of one event
-- binds [exception exception.type IOError]
arrayExists(x -> (
	(
		x.name = ?
		AND
		(
			arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value)
		)
	)
), s.events)
-- the bound level beside the level of the span
-- binds [^cache GET /cart zone us-east]
arrayExists(x -> (
	(
		x.name REGEXP ?
		AND
		s.name = ?
		AND
		(
			arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		)
	)
), s.events)
-- exists, ne and not_in on one link
-- binds [congo=t61rcWkgMzE zone zone zone us-east zone eu-west]
arrayExists(x -> (
	(
		x.trace_state != ''
		AND
		(x.trace_state != '' AND NOT (x.trace_state = ?))
		AND
		(
			(has(x.str_attributes.key, ?))
		)
		AND
		(
			(
				(has(x.str_attributes.key, ?))
			)
			AND NOT
			(
				arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value)
				OR
				arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value)
			)
		)
	)
), s.links)
-- an attribute of an event inside a predicate over one link searches every event
-- binds [00000000000000000000000000000f01 attempt 1]
arrayExists(x -> (
	(
		x.trace_id = ?
		AND
		(
			arrayExists(x -> arrayExists((key, value) -> key = ? AND value > ?, x.int_attributes.key, x.int_attributes.value), s.events)
		)
	)
), s.links)
-- every event is named, by a negation around a negated predicate
-- binds []
NOT (
	arrayExists(x -> (
		NOT (
			true
		)
	), s.events)
)
-- a level is bound only inside its own predicate
-- binds [exception retry]
(
	arrayExists(x -> (
		x.name = ?
	), s.events)
	AND
	arrayExists(x -> x.name = ?, s.events)
)

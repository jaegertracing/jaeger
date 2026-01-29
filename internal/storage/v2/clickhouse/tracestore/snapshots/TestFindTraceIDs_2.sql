SELECT DISTINCT
    s.trace_id,
    t.start,
    t.end
FROM spans s
LEFT JOIN trace_id_timestamps t ON s.trace_id = t.trace_id
WHERE 1=1
	AND s.service_name = ?
	AND s.name = ?
	AND s.duration >= ?
	AND s.duration <= ?
	AND s.start_time >= ?
	AND s.start_time <= ?
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_bool_attributes.key, s.resource_bool_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_bool_attributes.key, s.scope_bool_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_bool_attributes.key, s.event_bool_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.double_attributes.key, s.double_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_double_attributes.key, s.resource_double_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_double_attributes.key, s.scope_double_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_double_attributes.key, s.event_double_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_int_attributes.key, s.scope_int_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_int_attributes.key, s.event_int_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_complex_attributes.key, s.scope_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_complex_attributes.key, s.event_complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_complex_attributes.key, s.scope_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_complex_attributes.key, s.event_complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_complex_attributes.key, s.scope_complex_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_complex_attributes.key, s.event_complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_str_attributes.key, s.scope_str_attributes.value)
		OR 
		arrayExists((key, value) -> key = ? AND value = ?, s.event_str_attributes.key, s.event_str_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_double_attributes.key, s.resource_double_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.scope_int_attributes.key, s.scope_int_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)
	)
	AND (
		arrayExists((key, value) -> key = ? AND value = ?, s.event_str_attributes.key, s.event_str_attributes.value)
	)
LIMIT ?

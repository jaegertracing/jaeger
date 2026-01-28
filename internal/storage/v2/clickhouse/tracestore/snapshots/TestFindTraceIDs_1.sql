SELECT
    attribute_key,
    type,
    level
FROM
    attribute_metadata
WHERE
	attribute_key IN (?, ?, ?, ?, ?, ?, ?, ?)
GROUP BY
	attribute_key,
	type,
	level

package dbmodel

// AttributeMetadata represents metadata about an attribute stored in ClickHouse.
// This is populated by the attribute_metadata materialized view which tracks
// all unique (attribute_key, type, level) tuples observed in the spans table.
//
// The same attribute key can have multiple entries with different types or levels.
// For example, "http.status" might appear as both type="int" and type="str" if
// different spans store it with different types.
type AttributeMetadata struct {
	// AttributeKey is the name of the attribute (e.g., "http.status", "service.name")
	AttributeKey string
	// Type is the data type of the attribute value.
	// One of: "bool", "double", "int", "str", "bytes", "map", "slice"
	Type string
	// Level is the scope level where this attribute appears.
	// One of: "span", "resource", "scope"
	Level string
}

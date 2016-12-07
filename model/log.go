package model

// Log describes a micro-log entry that consists of a timestamp and one or more key-value fields
type Log struct {
	Timestamp int64      `json:"timestamp"`
	Fields    []KeyValue `json:"fields"`
}

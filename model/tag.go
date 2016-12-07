package model

import "sort"

// Tag is a key-value attribute associated with a Span
type Tag KeyValue

type tagsByKeyThenTypeThenValue []Tag

func (t tagsByKeyThenTypeThenValue) Len() int      { return len(t) }
func (t tagsByKeyThenTypeThenValue) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t tagsByKeyThenTypeThenValue) Less(i, j int) bool {
	one := KeyValue(t[i])
	two := KeyValue(t[j])
	return IsLess(&one, &two)
}

// SortTags sorts a slice of tags in-place by key, then by value type, then by value.
func SortTags(tags []Tag) {
	sort.Sort(tagsByKeyThenTypeThenValue(tags))
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// BoolQuery composes other queries under must / should / must_not clauses. To
// reproduce the wire format the storage layer previously produced via olivere's
// BoolQuery, a clause holding a single query renders as that query's object and a
// clause holding several renders as an array; an empty clause is omitted. No
// boost or adjust_pure_negative is emitted unless a caller needs it (none do).
type BoolQuery struct {
	must    []Query
	filter  []Query
	should  []Query
	mustNot []Query
}

// NewBoolQuery creates an empty BoolQuery.
func NewBoolQuery() *BoolQuery {
	return &BoolQuery{}
}

// Must adds clauses that all must match.
func (q *BoolQuery) Must(queries ...Query) *BoolQuery {
	q.must = append(q.must, queries...)
	return q
}

// Filter adds clauses that all must match, in the non-scoring filter context.
func (q *BoolQuery) Filter(queries ...Query) *BoolQuery {
	q.filter = append(q.filter, queries...)
	return q
}

// Should adds clauses of which at least one should match.
func (q *BoolQuery) Should(queries ...Query) *BoolQuery {
	q.should = append(q.should, queries...)
	return q
}

// MustNot adds clauses that must not match.
func (q *BoolQuery) MustNot(queries ...Query) *BoolQuery {
	q.mustNot = append(q.mustNot, queries...)
	return q
}

func (q *BoolQuery) Source() (any, error) {
	boolClause := make(map[string]any)
	for _, c := range []struct {
		key     string
		clauses []Query
	}{
		{"must", q.must},
		{"filter", q.filter},
		{"should", q.should},
		{"must_not", q.mustNot},
	} {
		src, err := clauseSource(c.clauses)
		if err != nil {
			return nil, err
		}
		if src != nil {
			boolClause[c.key] = src
		}
	}
	return map[string]any{"bool": boolClause}, nil
}

// clauseSource renders a bool clause the way olivere did: nil when empty, the
// single query's source when there is one, an array otherwise.
func clauseSource(clauses []Query) (any, error) {
	switch len(clauses) {
	case 0:
		return nil, nil
	case 1:
		return clauses[0].Source()
	default:
		arr := make([]any, len(clauses))
		for i, c := range clauses {
			src, err := c.Source()
			if err != nil {
				return nil, err
			}
			arr[i] = src
		}
		return arr, nil
	}
}

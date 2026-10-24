// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"

	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// luceneReserved holds the characters Lucene's regexp syntax reads as operators when the query's
// optional operators are off, which is how every regexp query here is sent (RFC 0005 §5.3).
const luceneReserved = `.?+*|{}[]()"\`

// luceneClassReserved holds the characters that Lucene reads as syntax inside a character class.
const luceneClassReserved = `\]-^`

// errAnchorInside is the refusal for an anchor that is not the first or the last element of the
// pattern. Lucene matches a whole term and has no anchor syntax, so an anchor at either end lowers
// to leaving out the wildcard on that side, and one anywhere else has nothing to lower to.
var errAnchorInside = fmt.Errorf("%w: it matches a whole term, so an anchor is honored only as the first or the last element of the pattern", tracestore.ErrFilterUnsupported)

// forThisEngine builds the match for this engine's regexp query, or refuses a pattern it cannot
// express (RFC 0005 §5.3). Lucene reads regexp text in its own dialect, so the pattern is parsed and
// luceneRegex writes the query from the tree rather than from the text. The optional operators of
// the dialect (&, @, ~, <n-m>, # and <identifier>) are switched off with flags NONE, so that an
// address like `@example.com` is the text the filter asked for rather than an operator.
func forThisEngine(pattern string) (valueMatch, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		// The boundary parses every pattern before a backend sees it, so this is unreachable
		// through the API; it is answered rather than ignored because this function also
		// answers for any text it is handed.
		return nil, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
	}
	lowered, err := luceneRegex(parsed)
	if err != nil {
		return nil, err
	}
	return func(field string) esquery.Query {
		return esquery.NewRegexpQuery(field, lowered).Flags("NONE")
	}, nil
}

// luceneRegex writes a parsed RE2 pattern in Lucene's regexp syntax, wrapped for a whole-term match.
// The query matches a whole indexed term while a filter's pattern matches anywhere in the value
// unless it anchors itself, so the pattern is grouped and a wildcard is put on each side the pattern
// leaves open: `GET` becomes `.*(GET).*`, `^GET` becomes `(GET).*`, and `^GET$` becomes `(GET)`. The
// group keeps a top-level alternation from swallowing the wildcards.
//
// The parser has already resolved everything the two dialects spell differently. A Perl shorthand
// such as `\d` and a Unicode class such as `\pL` are rune ranges; a case fold, whether it covers
// the whole pattern or one branch of it, is folded into the ranges of each class and into a flag on
// each literal, which the writer expands into the set of the letter's case equivalents, the Kelvin
// sign for `k` and the long s for `s` among them; and a non-capturing group is gone. Lucene has no
// anchors, so one anywhere but the ends of the pattern is refused, as is a class holding no
// character, which it has no spelling for under the flags the query is sent with. The boundary
// refuses both, and the line anchors of (?m), before a pattern reaches here, so those refusals
// answer only a direct call.
func luceneRegex(parsed *syntax.Regexp) (string, error) {
	parts, atStart, atEnd := tracestore.SplitPatternAnchors(parsed)
	var w luceneWriter
	w.WriteString("(")
	if len(parts) == 1 && parts[0].Op == syntax.OpAlternate {
		// The group written here already holds the alternation, so its branches need no group
		// of their own.
		w.alternation(parts[0])
	} else {
		w.sequence(parts)
	}
	w.WriteString(")")
	if w.err != nil {
		return "", w.err
	}
	lowered := w.String()
	if !atStart {
		lowered = ".*" + lowered
	}
	if !atEnd {
		lowered += ".*"
	}
	return lowered, nil
}

// luceneWriter spells a syntax tree in Lucene's regexp syntax. The first node it cannot spell sets
// err, and the text written after that is discarded by the caller.
type luceneWriter struct {
	strings.Builder
	err error
}

func (w *luceneWriter) sequence(parts []*syntax.Regexp) {
	for _, part := range parts {
		w.node(part)
	}
}

// node writes one node in a form that can stand in a sequence: an alternation is grouped.
func (w *luceneWriter) node(re *syntax.Regexp) {
	switch re.Op {
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			w.literal(r, re.Flags&syntax.FoldCase != 0)
		}
	case syntax.OpCharClass:
		w.class(re.Rune)
	case syntax.OpAnyCharNotNL:
		// Lucene's `.` matches a newline, which RE2's does not without (?s).
		w.WriteString("[^\n]")
	case syntax.OpAnyChar:
		w.WriteString(".")
	case syntax.OpEmptyMatch:
		w.WriteString("()")
	case syntax.OpCapture:
		w.WriteString("(")
		if re.Sub[0].Op == syntax.OpAlternate {
			w.alternation(re.Sub[0])
		} else {
			w.node(re.Sub[0])
		}
		w.WriteString(")")
	case syntax.OpConcat:
		w.sequence(re.Sub)
	case syntax.OpAlternate:
		w.WriteString("(")
		w.alternation(re)
		w.WriteString(")")
	case syntax.OpStar:
		w.atom(re.Sub[0])
		w.WriteString("*")
	case syntax.OpPlus:
		w.atom(re.Sub[0])
		w.WriteString("+")
	case syntax.OpQuest:
		w.atom(re.Sub[0])
		w.WriteString("?")
	case syntax.OpRepeat:
		w.atom(re.Sub[0])
		w.repeat(re.Min, re.Max)
	case syntax.OpBeginText, syntax.OpEndText:
		w.fail(errAnchorInside)
	default:
		// The line anchors and the word boundaries are what is left.
		w.fail(fmt.Errorf("%w: it has no form for %v", tracestore.ErrFilterUnsupported, re.Op))
	}
}

func (w *luceneWriter) alternation(re *syntax.Regexp) {
	for i, branch := range re.Sub {
		if i > 0 {
			w.WriteString("|")
		}
		w.node(branch)
	}
}

// atom writes the operand of a quantifier, grouped unless it is already one character or one group,
// because Lucene applies a quantifier to the single expression before it, as RE2 does, and a literal
// of several runes is several expressions once written out.
func (w *luceneWriter) atom(re *syntax.Regexp) {
	switch {
	case re.Op == syntax.OpLiteral && len(re.Rune) == 1,
		re.Op == syntax.OpCharClass, re.Op == syntax.OpAnyCharNotNL, re.Op == syntax.OpAnyChar,
		re.Op == syntax.OpCapture, re.Op == syntax.OpAlternate:
		w.node(re)
	default:
		w.WriteString("(")
		w.node(re)
		w.WriteString(")")
	}
}

func (w *luceneWriter) repeat(minCount, maxCount int) {
	switch {
	case maxCount < 0:
		fmt.Fprintf(w, "{%d,}", minCount)
	case minCount == maxCount:
		fmt.Fprintf(w, "{%d}", minCount)
	default:
		fmt.Fprintf(w, "{%d,%d}", minCount, maxCount)
	}
}

// literal writes one rune, as the class of its case equivalents when the pattern folds case there.
func (w *luceneWriter) literal(r rune, folded bool) {
	if utf16.IsSurrogate(r) {
		w.fail(fmt.Errorf("%w: it has no form for the surrogate code point %U", tracestore.ErrFilterUnsupported, r))
		return
	}
	if !folded {
		w.escaped(r, luceneReserved)
		return
	}
	orbit := []rune{r}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		orbit = append(orbit, f)
	}
	if len(orbit) == 1 {
		w.escaped(r, luceneReserved)
		return
	}
	slices.Sort(orbit)
	w.WriteString("[")
	for _, member := range orbit {
		w.escaped(member, luceneClassReserved)
	}
	w.WriteString("]")
}

// everyRune is the class of every code point text can hold, which Lucene spells `.`.
var everyRune = tracestore.WithoutSurrogates([]rune{0, unicode.MaxRune})

// class writes a character class from the parser's rune ranges, which come in pairs of inclusive
// bounds, with the surrogates taken out first. A class that runs from the first rune to the last is
// one the parser has complemented, and it is written back as the negation of the gaps between its
// ranges, which is both shorter and what was written. A class of every rune is written as the
// any-character operator, and a class of none is refused, since Lucene has no spelling for it.
func (w *luceneWriter) class(ranges []rune) {
	ranges = tracestore.WithoutSurrogates(ranges)
	switch {
	case len(ranges) == 0:
		w.fail(fmt.Errorf("%w: it has no form for a class that matches nothing", tracestore.ErrFilterUnsupported))
		return
	case slices.Equal(ranges, everyRune):
		w.WriteString(".")
		return
	}
	w.WriteString("[")
	if ranges[0] == 0 && ranges[len(ranges)-1] == unicode.MaxRune {
		w.WriteString("^")
		gaps := make([]rune, 0, len(ranges))
		for i := 1; i+1 < len(ranges); i += 2 {
			gaps = append(gaps, ranges[i]+1, ranges[i+1]-1)
		}
		ranges = tracestore.WithoutSurrogates(gaps)
	}
	for i := 0; i < len(ranges); i += 2 {
		w.classRange(ranges[i], ranges[i+1])
	}
	w.WriteString("]")
}

// classRange writes the members from lo to hi. The Lucene parsers in the supported engines agree
// that an escaped character is a member and that the upper bound of a range may be escaped, but
// only the older ones let an escaped character open a range, so a lower bound that needs an escape
// is written as a member of its own and the range starts after it. A bare `-` after a member is the
// range operator in every one of them, so it is escaped wherever it stands.
func (w *luceneWriter) classRange(lo, hi rune) {
	for lo < hi && strings.ContainsRune(luceneClassReserved, lo) {
		w.escaped(lo, luceneClassReserved)
		lo++
	}
	w.escaped(lo, luceneClassReserved)
	if lo < hi {
		w.WriteString("-")
		w.escaped(hi, luceneClassReserved)
	}
}

func (w *luceneWriter) escaped(r rune, reserved string) {
	if strings.ContainsRune(reserved, r) {
		w.WriteString(`\`)
	}
	w.WriteRune(r)
}

func (w *luceneWriter) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TestLuceneRegex pins the Lucene spelling of each construct the parser can produce. Whether Lucene
// reads each spelling as meant is checked against a running cluster by the filter battery of the
// storage integration suite. The letters beyond ASCII are written as escapes so that the file reads
// the same in every editor: \u017f is the long s, \u212a the Kelvin sign.
func TestLuceneRegex(t *testing.T) {
	for pattern, want := range map[string]string{
		// An anchor lowers to leaving out the wildcard on its side, and a dollar that is escaped is
		// a literal, which Lucene reads as one without an escape.
		"GET":     ".*(GET).*",
		"^GET":    "(GET).*",
		`\.json$`: `.*(\.json)`,
		"^GET$":   "(GET)",
		"^^GET$$": "(GET)",
		"^":       "().*",
		"^$":      "()",
		"":        ".*(()).*",
		`GET\$`:   ".*(GET$).*",
		// Alternation and groups. The parser factors a common prefix out of an alternation and
		// drops a non-capturing group, so those spell the same as their plain forms.
		"GET|POST":          ".*(GET|POST).*",
		"cart|checkout":     ".*(c(art|heckout)).*",
		"(?:GET|POST) /api": ".*((GET|POST) /api).*",
		"(GET|POST) /api":   ".*((GET|POST) /api).*",
		"(ab)+c":            ".*((ab)+c).*",
		"a|":                ".*(a|()).*",
		"()":                ".*((())).*",
		// A quantifier applies to one character or one group, so a literal of several runes is
		// grouped before it.
		"ab*":       ".*(ab*).*",
		"(?:ab)*":   ".*((ab)*).*",
		"x{3}":      ".*(x{3}).*",
		"x{2,}":     ".*(x{2,}).*",
		"x{2,5}":    ".*(x{2,5}).*",
		"(?:ab){2}": ".*((ab){2}).*",
		"[a-z]+":    ".*([a-z]+).*",
		".?":        ".*([^\n]?).*",
		"(?s).":     ".*(.).*",
		"(?:a*)*":   ".*((a*)*).*",
		"(?i)k+":    ".*([Kk\u212a]+).*",
		// A class is written from the parsed ranges, so a shorthand and the class it stands for
		// spell the same, and a complemented class is written back as a negation.
		`\d`:                ".*([0-9]).*",
		"[0-9]":             ".*([0-9]).*",
		`\w`:                ".*([0-9A-Z_a-z]).*",
		`\s`:                ".*([\t-\n\f-\r ]).*",
		"[^0-9]":            ".*([^0-9]).*",
		"[^/]":              ".*([^/]).*",
		`[^\n]`:             ".*([^\n]).*",
		"[a-c-]":            `.*([\-a-c]).*`,
		"[+-]":              `.*([+\-]).*`,
		"[--/]":             `.*([\-.-/]).*`,
		`[+\-./]`:           `.*([+\-.-/]).*`,
		`[ \-.]`:            `.*([ \-.]).*`,
		`[^ \-.]`:           `.*([^ \-.]).*`,
		`[,\-./]`:           `.*([,-/]).*`,
		`[+--]`:             `.*([+-\-]).*`,
		`[\]a]`:             `.*([\]a]).*`,
		`[\]-a]`:            `.*([\]\^_-a]).*`,
		`[\\-_]`:            `.*([\\\]\^_]).*`,
		`[Z-\]]`:            `.*([Z-\]]).*`,
		`[Z-\\]`:            `.*([Z-\\]).*`,
		`[\\]`:              `.*(\\).*`,
		`[^\x00-\x5B]`:      ".*([\\\\\\]\\^_-\ud7ff\ue000-\U0010FFFF]).*",
		"[^^]":              `.*([^\^]).*`,
		"[^-^]":             `.*([^\-\^]).*`,
		"[^a^]":             `.*([^\^a]).*`,
		"[a^]":              `.*([\^a]).*`,
		"[[a]":              ".*([[a]).*",
		"(?i)[[-_]":         `.*([[-_]).*`,
		"[\x00-\U0010FFFF]": ".*(.).*",
		`a[\s\S]b`:          ".*(a.b).*",
		`foo[\s\S]*bar`:     ".*(foo.*bar).*",
		`^[\s\S]+$`:         "(.+)",
		// A surrogate code point is left out of a class, since text holds none and Lucene cannot
		// spell one.
		`[^\x{D800}-\x{DFFF}]`: ".*(.).*",
		`[\x{DFFF}-\x{E000}]`:  ".*([\ue000]).*",
		`[\x{D7FF}-\x{E000}]`:  ".*([\ud7ff\ue000]).*",
		`[^\x{D7FF}-\x{E000}]`: ".*([^\ud7ff\ue000]).*",
		`[\x{D000}-\x{F000}]`:  ".*([\ud000-\ud7ff\ue000-\uf000]).*",
		// A folded letter is written as the set of its case equivalents, the Kelvin sign and the
		// long s among them, so the engine folds what RE2 folds and nothing else.
		"(?i)get":        ".*([Gg][Ee][Tt]).*",
		"(?i)k":          ".*([Kk\u212a]).*",
		"(?i)s":          ".*([Ss\u017f]).*",
		"(?i)9":          ".*(9).*",
		"(?i)[a-z]+":     ".*([A-Za-z\u017f\u212a]+).*",
		"(?i)[a-zA-Z]":   ".*([A-Za-z\u017f\u212a]).*",
		"(?i)caf\u00e9":  ".*([Cc][Aa][Ff][\u00c9\u00e9]).*",
		"[Gg]et":         ".*([Gg]et).*",
		"get|(?i)post":   ".*(get|[Pp][Oo][Ss\u017f][Tt]).*",
		"(?i)x|(?-i)y":   ".*([Xx-y]).*",
		"(?i:get) /api":  ".*([Gg][Ee][Tt] /api).*",
		"(?i)^po\u017ft": "([Pp][Oo][Ss\u017f][Tt]).*",
		// The characters Lucene reads as syntax are escaped, and the rest are written as they are.
		`a\.b`:         `.*(a\.b).*`,
		`\(\)\[\]\{\}`: `.*(\(\)\[\]\{\}).*`,
		`\?\*\+\|`:     `.*(\?\*\+\|).*`,
		`"x"`:          `.*(\"x\").*`,
		`a\\d`:         `.*(a\\d).*`,
		"@example.com": ".*(@example[^\n]com).*",
		"a&b~c#d<e>f":  ".*(a&b~c#d<e>f).*",
		"\u00fcber":    ".*(\u00fcber).*",
	} {
		t.Run(pattern, func(t *testing.T) {
			parsed, err := syntax.Parse(pattern, syntax.Perl)
			require.NoError(t, err)
			got, err := luceneRegex(parsed)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestLuceneRegexRefusals(t *testing.T) {
	for pattern, want := range map[string]string{
		"GET^x":                 "an anchor is honored only as the first or the last element",
		"^GET|POST":             "an anchor is honored only as the first or the last element",
		"^GET|^POST":            "an anchor is honored only as the first or the last element",
		"(^GET)":                "an anchor is honored only as the first or the last element",
		"(?:^GET)+":             "an anchor is honored only as the first or the last element",
		"(?m)^GET":              "it has no form for BeginLine",
		"(?m)GET$":              "it has no form for EndLine",
		`\bGET`:                 "it has no form for WordBoundary",
		`\BGET`:                 "it has no form for NoWordBoundary",
		`[^\x00-\x{10FFFF}]`:    "a class that matches nothing",
		`GET[^\x00-\x{10FFFF}]`: "a class that matches nothing",
		`[\x{D800}-\x{DBFF}]`:   "a class that matches nothing",
		`GET\x{D800}`:           "the surrogate code point U+D800",
	} {
		t.Run(pattern, func(t *testing.T) {
			parsed, err := syntax.Parse(pattern, syntax.Perl)
			require.NoError(t, err)
			_, err = luceneRegex(parsed)
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.ErrorContains(t, err, want)
		})
	}
}

// TestForThisEngineInvalid covers the one refusal the API cannot reach, since the boundary parses
// every pattern first.
func TestForThisEngineInvalid(t *testing.T) {
	_, err := forThisEngine("GET (")
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

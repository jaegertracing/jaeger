// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
)

// TraceID is a random 128bit identifier for a trace
// type TraceID struct {
// 	Low  uint64 `json:"lo"`
// 	High uint64 `json:"hi"`
// }

// SpanID is a random 64bit identifier for a span
type SpanID uint64

// ------- TraceID -------

// NewTraceID creates a new TraceID from two 64bit unsigned ints.
func NewTraceID(high, low uint64) TraceID {
	return TraceID{High: high, Low: low}
}

func (t TraceID) String() string {
	if t.High == 0 {
		return fmt.Sprintf("%x", t.Low)
	}
	return fmt.Sprintf("%x%016x", t.High, t.Low)
}

// TraceIDFromString creates a TraceID from a hexadecimal string
func TraceIDFromString(s string) (TraceID, error) {
	var hi, lo uint64
	var err error
	if len(s) > 32 {
		return TraceID{}, fmt.Errorf("TraceID cannot be longer than 32 hex characters: %s", s)
	} else if len(s) > 16 {
		hiLen := len(s) - 16
		if hi, err = strconv.ParseUint(s[0:hiLen], 16, 64); err != nil {
			return TraceID{}, err
		}
		if lo, err = strconv.ParseUint(s[hiLen:], 16, 64); err != nil {
			return TraceID{}, err
		}
	} else {
		if lo, err = strconv.ParseUint(s, 16, 64); err != nil {
			return TraceID{}, err
		}
	}
	return TraceID{High: hi, Low: lo}, nil
}

// MarshalText is called by encoding/json, which we do not want people to use.
func (t TraceID) MarshalText() ([]byte, error) {
	return nil, fmt.Errorf("unsupported method TraceID.MarshalText; please use github.com/gogo/protobuf/jsonpb for marshalling")
}

// UnmarshalText is called by encoding/json, which we do not want people to use.
func (t *TraceID) UnmarshalText(text []byte) error {
	return fmt.Errorf("unsupported method TraceID.UnmarshalText; please use github.com/gogo/protobuf/jsonpb for marshalling")
}

// MarshalJSONPB renders trace id as a single hex string.
func (t TraceID) MarshalJSONPB(*jsonpb.Marshaler) ([]byte, error) {
	var b strings.Builder
	s := t.String()
	b.Grow(2 + len(s))
	b.WriteByte('"')
	b.WriteString(s)
	b.WriteByte('"')
	return []byte(b.String()), nil
}

// UnmarshalJSONPB populates TraceID from a quoted hex string. Called by gogo/protobuf/jsonpb.
func (t *TraceID) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, b []byte) error {
	if len(b) < 3 {
		return fmt.Errorf("TraceID JSON string cannot be shorter than 3 chars: '%s'", string(b))
	}
	if b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("TraceID JSON string must be enclosed in quotes: '%s'", string(b))
	}
	q, err := TraceIDFromString(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	*t = q
	return nil
}

// ------- SpanID -------

// NewSpanID creates a new SpanID from a 64bit unsigned int.
func NewSpanID(v uint64) SpanID {
	return SpanID(v)
}

func (s SpanID) String() string {
	return fmt.Sprintf("%x", uint64(s))
}

// SpanIDFromString creates a SpanID from a hexadecimal string
func SpanIDFromString(s string) (SpanID, error) {
	if len(s) > 16 {
		return SpanID(0), fmt.Errorf("SpanID cannot be longer than 16 hex characters: %s", s)
	}
	id, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return SpanID(0), err
	}
	return SpanID(id), nil
}

// MarshalText allows SpanID to serialize itself in JSON as a string.
func (s SpanID) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText allows SpanID to deserialize itself from a JSON string.
func (s *SpanID) UnmarshalText(text []byte) error {
	q, err := SpanIDFromString(string(text))
	if err != nil {
		return err
	}
	*s = q
	return nil
}

// MarshalJSON renders span id as a single hex string. The value is returned enclosed in quotes.
func (s SpanID) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	str := s.String()
	b.Grow(2 + len(str))
	b.WriteByte('"')
	b.WriteString(str)
	b.WriteByte('"')
	return []byte(b.String()), nil
}

// UnmarshalJSON populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// There appears to be a bug in gogoproto, as this function is only called for numeric values.
// https://github.com/gogo/protobuf/issues/411#issuecomment-393856837
func (s *SpanID) UnmarshalJSON(b []byte) error {
	q, err := SpanIDFromString(string(b))
	if err != nil {
		return err
	}
	*s = q
	return nil
}

// UnmarshalJSONPB populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// The input value is a quoted string.
func (s *SpanID) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, b []byte) error {
	if len(b) < 3 {
		return fmt.Errorf("SpanID JSON string cannot be shorter than 3 chars: %s", string(b))
	}
	if b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("SpanID JSON string must be enclosed in quotes: %s", string(b))
	}
	return s.UnmarshalJSON(b[1 : len(b)-1])
}

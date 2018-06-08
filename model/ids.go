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
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/gogo/protobuf/jsonpb"
)

// TraceID is a random 128bit identifier for a trace
type TraceID struct {
	Low  uint64 `json:"lo"`
	High uint64 `json:"hi"`
}

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

func (t *TraceID) Size() int {
	return 16
}

func (t *TraceID) MarshalTo(data []byte) (n int, err error) {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(t.High))
	binary.BigEndian.PutUint64(b[8:], uint64(t.Low))
	return marshalBytes(data, b[:])
}

func (t *TraceID) Unmarshal(data []byte) error {
	if len(data) < 16 {
		return fmt.Errorf("buffer is too short")
	}
	t.High = binary.BigEndian.Uint64(data[:8])
	t.Low = binary.BigEndian.Uint64(data[8:])
	return nil
}

func marshalBytes(dst []byte, src []byte) (n int, err error) {
	if len(dst) < len(src) {
		return 0, fmt.Errorf("buffer is too short")
	}
	return copy(dst, src), nil
}

// MarshalJSON renders trace id as base64 string.
func (t TraceID) MarshalJSON() ([]byte, error) {
	var b [16]byte
	t.MarshalTo(b[:]) // can only error on incorrect buffer size
	s := base64.StdEncoding.EncodeToString(b[:])
	return []byte(`"` + s + `"`), nil
}

func (t *TraceID) UnmarshalJSON(data []byte) error {
	s := string(data)
	if l := len(s); l > 2 && s[0] == '"' && s[l-1] == '"' {
		s = s[1 : l-1]
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("cannot unmarshal TraceID from string '%s': %v", string(data), err)
	}
	return t.Unmarshal(b)
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

// MarshalText is called by encoding/json, which we do not want people to use.
func (s SpanID) MarshalText() ([]byte, error) {
	return nil, fmt.Errorf("unsupported method SpanID.MarshalText; please use github.com/gogo/protobuf/jsonpb for marshalling")
}

// UnmarshalText is called by encoding/json, which we do not want people to use.
func (s *SpanID) UnmarshalText(text []byte) error {
	return fmt.Errorf("unsupported method SpanID.UnmarshalText; please use github.com/gogo/protobuf/jsonpb for marshalling")
}

// // UnmarshalJSON populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// // There appears to be a bug in gogoproto, as this function is only called for numeric values.
// // https://github.com/gogo/protobuf/issues/411#issuecomment-393856837
// func (s *SpanID) UnmarshalJSON(b []byte) error {
// 	q, err := SpanIDFromString(string(b))
// 	if err != nil {
// 		return err
// 	}
// 	*s = q
// 	return nil
// }

// // UnmarshalJSONPB populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// // The input value is a quoted string.
// func (s *SpanID) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, b []byte) error {
// 	if len(b) < 3 {
// 		return fmt.Errorf("SpanID JSON string cannot be shorter than 3 chars: %s", string(b))
// 	}
// 	if b[0] != '"' || b[len(b)-1] != '"' {
// 		return fmt.Errorf("SpanID JSON string must be enclosed in quotes: %s", string(b))
// 	}
// 	return s.UnmarshalJSON(b[1 : len(b)-1])
// }

func (s *SpanID) Size() int {
	return 8
}

func (s *SpanID) MarshalTo(data []byte) (n int, err error) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(*s))
	return marshalBytes(data, b[:])
}

func (s *SpanID) Unmarshal(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("buffer is too short")
	}
	*s = NewSpanID(binary.BigEndian.Uint64(data))
	return nil
}

// MarshalJSON renders span id as base64 string.
func (s SpanID) MarshalJSON() ([]byte, error) {
	var b [8]byte
	s.MarshalTo(b[:]) // can only error on incorrect buffer size
	str := base64.StdEncoding.EncodeToString(b[:])
	return []byte(`"` + str + `"`), nil
}

// UnmarshalJSON populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// There appears to be a bug in gogoproto, as this function is only called for numeric values.
// https://github.com/gogo/protobuf/issues/411#issuecomment-393856837
func (s *SpanID) UnmarshalJSON(data []byte) error {
	str := string(data)
	if l := len(str); l > 2 && str[0] == '"' && str[l-1] == '"' {
		str = str[1 : l-1]
	}
	b, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return fmt.Errorf("cannot unmarshal SpanID from string '%s': %v", string(data), err)
	}
	return s.Unmarshal(b)
}

// UnmarshalJSONPB populates SpanID from a quoted hex string. Called by gogo/protobuf/jsonpb.
// The input value is a quoted string.
func (s *SpanID) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, b []byte) error {
	return s.UnmarshalJSON(b)
}

package v1

import (
	"fmt"
	"io"
)

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (s *Span) UnmarshalGQL(v interface{}) error {
	fmt.Println("span unmarshaller")
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (s *Span) MarshalGQL(w io.Writer) {
	fmt.Println("span marshaller")
	jsonpb := JSONPb{}
	json, err := jsonpb.Marshal(s)
	if err != nil {
		fmt.Println("Error while marshalling")
	}
	fmt.Println(string(json))
	_, err = w.Write(json)
	if err != nil {
		fmt.Println("Error while writing JSON")
	}
}

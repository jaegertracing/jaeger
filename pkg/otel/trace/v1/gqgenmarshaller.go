package v1

import (
	"fmt"
	"io"
)

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (s *Span) UnmarshalGQL(v interface{}) error {
	fmt.Println("\n\n go gen unmarshaller")
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (s Span) MarshalGQL(w io.Writer) {
	pb := JSONPb{}

	err := pb.marshalTo(w, s)
	if err != nil {
		fmt.Println("Error while marshalling")
	}
}

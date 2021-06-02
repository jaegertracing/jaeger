package v1

import (
	"fmt"
	"io"
)

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (rss *ResourceSpans) UnmarshalGQL(v interface{}) error {
	fmt.Println("resource spans unmarshaller")
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (rss *ResourceSpans) MarshalGQL(w io.Writer) {
	fmt.Println("resource spans marshaller")
	jsonpb := JSONPb{}
	json, err := jsonpb.Marshal(rss)
	if err != nil {
		fmt.Println("Error while marshalling")
	}
	fmt.Println(string(json))
	_, err = w.Write(json)
	if err != nil {
		fmt.Println("Error while writing JSON")
	}
}

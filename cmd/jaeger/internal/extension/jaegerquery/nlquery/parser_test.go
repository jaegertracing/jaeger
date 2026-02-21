package nlquery

import "testing"

func TestParseErrors(t *testing.T) {
	r, err := Parse("show me errors")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := r.Params.Attributes.Get("error")
	if !ok {
		t.Fatalf("expected error attribute to exist")
	}

	if val.Str() != "true" {
		t.Fatalf("expected error=true")
	}
}

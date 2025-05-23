package utils

import (
	"testing"
	"unicode/utf8"
)

func TestSanitizeUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Valid ASCII",
			input: "Hello, world!",
			want:  "Hello, world!",
		},
		{
			name:  "Valid UTF-8 multibyte",
			input: "こんにちは世界",
			want:  "こんにちは世界",
		},
		{
			name:  "Valid Emoji",
			input: "😃🚀🔥",
			want:  "😃🚀🔥",
		},
		{
			name:  "String with replacement rune",
			input: string([]byte{0xff, 0xfe, 0xfd}),
			want:  "���",
		},
		{
			name:  "Mixed valid and invalid",
			input: "Good\xffMorning",
			want:  "Good�Morning",
		},
		{
			name:  "Empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, err := SanitizeUTF8(tt.input)
			if err != nil {
				t.Errorf("SanitizeUTF8 failed: %v", err)
			}
			if !utf8.ValidString(sanitized) {
				t.Errorf("SanitizeUTF8 returned invalid UTF-8 string: %s, want %s", sanitized, tt.want)
			}
			if sanitized != tt.want {
				t.Errorf("SanitizeUTF8 gave not correct output: %s, want %s", sanitized, tt.want)
			}
		})
	}
}

package helper

import "strings"

// stripWhiteSpace removes all whitespace characters from a string
func StripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}

package testutils

import (
	"runtime"
	"strings"
)

// NormalizeErrorMessage converts Windows-specific error messages
// to their Unix equivalents for cross-platform test compatibility.
func NormalizeErrorMessage(msg string) string {
	if runtime.GOOS != "windows" {
		return msg
	}
	replacements := map[string]string{
		"The system cannot find the file specified.":                                      "no such file or directory",
		"The system cannot find the path specified.":                                      "no such file or directory",
		"The process cannot access the file because it is being used by another process.": "no such file or directory",
		"The filename, directory name, or volume label syntax is incorrect.":              "no such file or directory",
	}
	for windowsMsg, unixMsg := range replacements {
		msg = strings.ReplaceAll(msg, windowsMsg, unixMsg)
	}
	return msg
}

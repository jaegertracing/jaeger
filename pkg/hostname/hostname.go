package hostname

import (
	"fmt"
	"math/rand"
	"os"
)

// AsIdentifier uses the hostname of the os and postfixes a short random string to guarantee uniqueness
// The returned value is appropriate to use as a convenient unique identifier.
func AsIdentifier() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	buff := make([]byte, 8)
	_, err = rand.Read(buff)
	if err != nil {
		return "", err
	}

	return hostname + "-" + fmt.Sprintf("%2x", buff), nil
}

package mainlekfe

import (
	"fmt"
	"strings"
)

func main() {
	template := `CREATE KEYSPACE IF NOT EXISTS ${keyspace} WITH replication = ${replication};`
	
	// Define the replacements
	replacements := map[string]string{
		"${keyspace}":         "my_keyspace",
		"${replication}":      "{'class': 'SimpleStrategy', 'replication_factor': '1'}",
		// Add other placeholders as needed
	}

	// Replace each placeholder in the template
	for placeholder, value := range replacements {
		template = strings.ReplaceAll(template, placeholder, value)
	}

	fmt.Println(template)
}

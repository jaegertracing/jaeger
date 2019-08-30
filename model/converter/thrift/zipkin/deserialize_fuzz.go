// +build gofuzz

package zipkin

func FuzzDeserializeThrift(data []byte) int {
	_, err := DeserializeThrift(data)
	if err != nil {
		return 0
	}
	return 1
}

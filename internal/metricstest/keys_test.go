package metricstest

import (
	"testing"
)

func TestGetKey(t *testing.T) {
	// Test case 1: Basic case with a single tag
	name := "metricName"
	tags := map[string]string{"tag1": "value1"}
	expected := "metricName|tag1=value1"
	result := GetKey(name, tags, "|", "=")
	if result != expected {
		t.Errorf("Test case 1 failed. Expected: %s, Got: %s", expected, result)
	}

	// Test case 2: Multiple tags with sorting
	name = "metricName"
	tags = map[string]string{"tag2": "value2", "tag1": "value1"}
	expected = "metricName|tag1=value1|tag2=value2"
	result = GetKey(name, tags, "|", "=")
	if result != expected {
		t.Errorf("Test case 2 failed. Expected: %s, Got: %s", expected, result)
	}

	// Test case 3: Case with no tags
	name = "metricName"
	tags = map[string]string{}
	expected = "metricName"
	result = GetKey(name, tags, "|", "=")
	if result != expected {
		t.Errorf("Test case 3 failed. Expected: %s, Got: %s", expected, result)
	}

	// Test case 4: Case with multiple tags and special characters
	name = "metricName"
	tags = map[string]string{"tag2": "value 2", "tag1": "value!@#1"}
	expected = "metricName|tag1=value!@#1|tag2=value 2"
	result = GetKey(name, tags, "|", "=")
	if result != expected {
		t.Errorf("Test case 4 failed. Expected: %s, Got: %s", expected, result)
	}
}

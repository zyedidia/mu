package completer

import (
	"testing"
)

func check(expected, actual []string, t *testing.T) {
	if len(expected) != len(actual) {
		t.Fatal("size mismatch")
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Fatal("mismatch")
		}
	}
}

func TestFileCompleter(t *testing.T) {
	expected := []string{
		"./completer.go",
		"./completer_test.go",
	}
	actual := FileComplete("./co", ".")
	check(expected, actual, t)
}

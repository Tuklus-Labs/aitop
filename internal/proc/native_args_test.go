package proc

import (
	"reflect"
	"testing"
)

func TestParseNativeCmdlinePreservesSpacesAndEmptyArguments(t *testing.T) {
	raw := []byte("tool\x00hello world\x00\x00two words\x00")
	want := []string{"tool", "hello world", "", "two words"}
	if got := parseNativeCmdline(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("native argv framing changed: got=%q want=%q", got, want)
	}
}

func TestParseNativeCmdlineDoesNotInventArgumentsForEmptyInput(t *testing.T) {
	if got := parseNativeCmdline(nil); got != nil {
		t.Fatalf("empty native argv should be nil: got=%q", got)
	}
	if got := parseNativeCmdline([]byte{0}); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("one empty native argv should be retained: got=%q", got)
	}
}

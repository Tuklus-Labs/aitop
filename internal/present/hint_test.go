package present

import "testing"

// The owner segment of a systemd machine name is whichever account runs the
// fleet. Pinning it to one name meant every other account's rows painted the
// raw "machine-<owner>-" prefix, so the cases below deliberately use several.
func TestHintStripsMachineOwnerSegment(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"machine-gary-parlor", "parlor"},
		{"machine-alice-parlor", "parlor"},
		{"machine-bob-two-seat", "two-seat"},
		{"unknown", ""},
		{"parlor", "parlor"},
		{"", ""},
		// No resident half, so there is nothing to reduce to and the name
		// stands rather than being silently emptied.
		{"machine-solo", "machine-solo"},
	} {
		if g := Hint(c.in); g != c.want {
			t.Fatalf("hint-strips-owner-segment violated: Hint(%q) = %q, want %q", c.in, g, c.want)
		}
	}
}

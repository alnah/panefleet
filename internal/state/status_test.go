package state

import "testing"

func TestStatusParsingAndValidity(t *testing.T) {
	valid := []Status{StatusUnknown, StatusIdle, StatusRun, StatusWait, StatusDone, StatusError, StatusStale}
	for _, s := range valid {
		if !s.Valid() {
			t.Fatalf("status %s should be valid", s)
		}
		parsed, err := ParseStatus(string(s))
		if err != nil {
			t.Fatalf("ParseStatus(%s): %v", s, err)
		}
		if parsed != s {
			t.Fatalf("ParseStatus(%s) = %s", s, parsed)
		}
	}

	if Status("MYSTERY").Valid() {
		t.Fatalf("unexpected valid status for MYSTERY")
	}
	if _, err := ParseStatus("MYSTERY"); err == nil {
		t.Fatalf("expected parse error for invalid status")
	}
	parsed, err := ParseStatus("  RUN \t")
	if err != nil {
		t.Fatalf("ParseStatus with spaces: %v", err)
	}
	if parsed != StatusRun {
		t.Fatalf("ParseStatus with spaces = %s, want RUN", parsed)
	}
}

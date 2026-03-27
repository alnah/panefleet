package tmuxctl

import "testing"

func TestParseControlLine(t *testing.T) {
	cases := []struct {
		line string
		ok   bool
		kind string
		}{
			{line: "%window-add @1", ok: true, kind: "%window-add"},
			{line: "%session-changed $1", ok: true, kind: "%session-changed"},
			{line: "%window-pane-changed @1 %2", ok: true, kind: "%window-pane-changed"},
			{line: "%sessions-changed", ok: true, kind: "%sessions-changed"},
			{line: "%output %1 hello", ok: false, kind: ""},
			{line: "random text", ok: false, kind: ""},
			{line: "", ok: false, kind: ""},
		}
	for _, tc := range cases {
		ev, ok := ParseControlLine(tc.line)
		if ok != tc.ok {
			t.Fatalf("line=%q expected ok=%v got=%v", tc.line, tc.ok, ok)
		}
		if ok && ev.Kind != tc.kind {
			t.Fatalf("line=%q expected kind=%s got=%s", tc.line, tc.kind, ev.Kind)
		}
	}
}

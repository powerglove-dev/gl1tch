package console

// terminal_nl_test.go — unit tests for parseTerminalNL and termSplitsDesc.
// No tmux required. These run in milliseconds.

import (
	"testing"
)

// ── parseTerminalNL ───────────────────────────────────────────────────────────

func TestParseTerminalNL_GenericOpen(t *testing.T) {
	cases := []string{
		"open a terminal",
		"a terminal",
		"terminal",
		"a shell",
		"open a shell",
		"open a pane",
		"create a terminal",
		"start a shell",
		"launch a terminal",
		"new terminal",
		"give me a terminal",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			splits, ok := parseTerminalNL(input)
			if !ok {
				t.Fatalf("expected NL parse, got raw-command fallback")
			}
			if len(splits) != 1 {
				t.Fatalf("expected 1 split, got %d", len(splits))
			}
			sp := splits[0]
			if sp.pct != 25 || sp.vertical || sp.left || sp.cwd != "" {
				t.Errorf("expected default split, got pct=%d vertical=%v left=%v cwd=%q", sp.pct, sp.vertical, sp.left, sp.cwd)
			}
		})
	}
}

func TestParseTerminalNL_Percent(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"open a shell 50% width", 50},
		{"50% shell", 50},
		{"shell at 33%", 33},
		{"open a half-width terminal", 50},
		{"half terminal", 50},
		{"open a third terminal", 33},
		{"full width terminal", 90},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			splits, ok := parseTerminalNL(tc.input)
			if !ok {
				t.Fatalf("expected NL parse for %q", tc.input)
			}
			if splits[0].pct != tc.want {
				t.Errorf("pct: got %d, want %d", splits[0].pct, tc.want)
			}
		})
	}
}

func TestParseTerminalNL_Direction(t *testing.T) {
	cases := []struct {
		input    string
		vertical bool
		left     bool
	}{
		{"open a terminal at the bottom", true, false},
		{"bottom terminal", true, false},
		{"vertical split", true, false},
		{"terminal below", true, false},
		{"left terminal", false, true},
		{"open a terminal on the left", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			splits, ok := parseTerminalNL(tc.input)
			if !ok {
				t.Fatalf("expected NL parse for %q", tc.input)
			}
			sp := splits[0]
			if sp.vertical != tc.vertical {
				t.Errorf("vertical: got %v, want %v", sp.vertical, tc.vertical)
			}
			if sp.left != tc.left {
				t.Errorf("left: got %v, want %v", sp.left, tc.left)
			}
		})
	}
}

func TestParseTerminalNL_Count(t *testing.T) {
	cases := []struct {
		input string
		count int
	}{
		{"3 terminals", 3},
		{"open 3 shells", 3},
		{"2 panes", 2},
		{"open 5 terminals", 5},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			splits, ok := parseTerminalNL(tc.input)
			if !ok {
				t.Fatalf("expected NL parse for %q", tc.input)
			}
			if len(splits) != tc.count {
				t.Errorf("count: got %d, want %d", len(splits), tc.count)
			}
		})
	}
}

func TestParseTerminalNL_CWD(t *testing.T) {
	splits, ok := parseTerminalNL("open 3 shells and set cwd to ../project1 ../project2 ../project3")
	if !ok {
		t.Fatal("expected NL parse")
	}
	if len(splits) != 3 {
		t.Fatalf("expected 3 splits, got %d", len(splits))
	}
	want := []string{"../project1", "../project2", "../project3"}
	for i, s := range splits {
		if s.cwd != want[i] {
			t.Errorf("splits[%d].cwd = %q, want %q", i, s.cwd, want[i])
		}
	}
}

func TestParseTerminalNL_CWD_Single(t *testing.T) {
	splits, ok := parseTerminalNL("terminal in ./myproject")
	if !ok {
		t.Fatal("expected NL parse")
	}
	if len(splits) != 1 {
		t.Fatalf("expected 1 split, got %d", len(splits))
	}
	if splits[0].cwd != "./myproject" {
		t.Errorf("cwd: got %q, want %q", splits[0].cwd, "./myproject")
	}
}

func TestParseTerminalNL_CWD_ExpandsCount(t *testing.T) {
	// 3 cwds but count not stated → count should become 3.
	splits, ok := parseTerminalNL("cwd to /tmp/a /tmp/b /tmp/c")
	if !ok {
		t.Fatal("expected NL parse")
	}
	if len(splits) != 3 {
		t.Fatalf("expected 3 splits (one per cwd), got %d", len(splits))
	}
}

func TestParseTerminalNL_RawCommand_NotTriggered(t *testing.T) {
	// These should NOT be NL-parsed — they are raw shell commands.
	cases := []string{
		"nvim .",
		"python3 script.py",
		"bash -c 'echo hi'",
		"htop",
		"ssh user@host",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, ok := parseTerminalNL(input)
			if ok {
				t.Errorf("parseTerminalNL(%q) returned NL=true; expected raw-command fallback", input)
			}
		})
	}
}

func TestParseTerminalNL_Combined(t *testing.T) {
	// "open 3 shells 50% width at the bottom"
	splits, ok := parseTerminalNL("open 3 shells 50% width at the bottom")
	if !ok {
		t.Fatal("expected NL parse")
	}
	if len(splits) != 3 {
		t.Fatalf("expected 3 splits, got %d", len(splits))
	}
	for i, s := range splits {
		if s.pct != 50 {
			t.Errorf("splits[%d].pct = %d, want 50", i, s.pct)
		}
		if !s.vertical {
			t.Errorf("splits[%d].vertical = false, want true", i)
		}
	}
}

// ── termSplitsDesc ────────────────────────────────────────────────────────────

func TestTermSplitsDesc_Single_Default(t *testing.T) {
	s := termSplitsDesc([]termSplit{{pct: 25}})
	if s != "opening terminal split." {
		t.Errorf("got %q", s)
	}
}

func TestTermSplitsDesc_Single_Percent(t *testing.T) {
	s := termSplitsDesc([]termSplit{{pct: 50}})
	if s != "opening 50% terminal split." {
		t.Errorf("got %q", s)
	}
}

func TestTermSplitsDesc_Single_WithCWD(t *testing.T) {
	s := termSplitsDesc([]termSplit{{pct: 25, cwd: "./myproject"}})
	if s != "opening terminal in ./myproject." {
		t.Errorf("got %q", s)
	}
}

func TestTermSplitsDesc_Single_Bottom(t *testing.T) {
	s := termSplitsDesc([]termSplit{{pct: 25, vertical: true}})
	if s != "opening bottom terminal split." {
		t.Errorf("got %q", s)
	}
}

func TestTermSplitsDesc_Multi_WithCWDs(t *testing.T) {
	splits := []termSplit{
		{pct: 25, cwd: "../project1"},
		{pct: 25, cwd: "../project2"},
		{pct: 25, cwd: "../project3"},
	}
	s := termSplitsDesc(splits)
	if s != "opening 3 terminals: ../project1, ../project2, ../project3" {
		t.Errorf("got %q", s)
	}
}

func TestTermSplitsDesc_Multi_NoCWD(t *testing.T) {
	splits := []termSplit{{pct: 25}, {pct: 25}, {pct: 25}}
	s := termSplitsDesc(splits)
	if s != "opening 3 terminals." {
		t.Errorf("got %q", s)
	}
}

// ── termSplit.tmuxArgs ────────────────────────────────────────────────────────

func TestTermSplit_TmuxArgs_Default(t *testing.T) {
	args := (termSplit{pct: 25}).tmuxArgs()
	want := []string{"split-window", "-h", "-p", "25"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d %v, want %d %v", len(args), args, len(want), want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], want[i])
		}
	}
}

func TestTermSplit_TmuxArgs_Vertical50WithCWD(t *testing.T) {
	args := (termSplit{pct: 50, vertical: true, cwd: "/tmp/foo"}).tmuxArgs()
	want := []string{"split-window", "-v", "-p", "50", "-c", "/tmp/foo"}
	if len(args) != len(want) {
		t.Fatalf("len: got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], want[i])
		}
	}
}

func TestTermSplit_TmuxArgs_LeftSplit(t *testing.T) {
	args := (termSplit{pct: 30, left: true}).tmuxArgs()
	want := []string{"split-window", "-h", "-b", "-p", "30"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], want[i])
		}
	}
}

package cli_test

import (
	"reflect"
	"testing"

	"github.com/mickamy/subset/internal/cli"
)

func TestSplitArgs(t *testing.T) {
	t.Parallel()

	isBool := func(name string) bool {
		return name == "plan" || name == "verbose"
	}

	cases := []struct {
		name      string
		args      []string
		wantFlags []string
		wantPos   []string
	}{
		{
			name:      "empty",
			args:      []string{},
			wantFlags: nil,
			wantPos:   nil,
		},
		{
			name:      "pure positional",
			args:      []string{"foo", "bar"},
			wantFlags: nil,
			wantPos:   []string{"foo", "bar"},
		},
		{
			name:      "value flag space-separated",
			args:      []string{"--where", "id = 1"},
			wantFlags: []string{"--where", "id = 1"},
			wantPos:   nil,
		},
		{
			name:      "value flag equals form",
			args:      []string{"--id=42"},
			wantFlags: []string{"--id=42"},
			wantPos:   nil,
		},
		{
			name:      "bool flag does not consume next token",
			args:      []string{"--plan", "table"},
			wantFlags: []string{"--plan"},
			wantPos:   []string{"table"},
		},
		{
			name:      "flag after positional",
			args:      []string{"dsn", "table", "--id", "1"},
			wantFlags: []string{"--id", "1"},
			wantPos:   []string{"dsn", "table"},
		},
		{
			name:      "mixed positional and flags",
			args:      []string{"dsn", "--plan", "table", "--id=42"},
			wantFlags: []string{"--plan", "--id=42"},
			wantPos:   []string{"dsn", "table"},
		},
		{
			name:      "double dash terminates flag parsing",
			args:      []string{"dsn", "--", "--not-a-flag", "more"},
			wantFlags: nil,
			wantPos:   []string{"dsn", "--not-a-flag", "more"},
		},
		{
			name:      "single-char flag treated as flag",
			args:      []string{"-h"},
			wantFlags: []string{"-h"},
			wantPos:   nil,
		},
		{
			name:      "bare dash is positional",
			args:      []string{"-"},
			wantFlags: nil,
			wantPos:   []string{"-"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotFlags, gotPos := cli.SplitArgs(tc.args, isBool)
			if !reflect.DeepEqual(gotFlags, tc.wantFlags) {
				t.Errorf("flags = %#v; want %#v", gotFlags, tc.wantFlags)
			}
			if !reflect.DeepEqual(gotPos, tc.wantPos) {
				t.Errorf("positional = %#v; want %#v", gotPos, tc.wantPos)
			}
		})
	}
}

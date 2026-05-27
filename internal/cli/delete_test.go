package cli_test

import (
	"bytes"
	"testing"

	"github.com/mickamy/subset/internal/cli"
	"github.com/mickamy/subset/internal/exit"
)

// TestRunDelete_ArgErrors covers the argument-validation and early-failure
// paths that never reach the database, so they need no integration fixture.
func TestRunDelete_ArgErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing dsn and table", []string{"delete"}, exit.Usage},
		{"too many positionals", []string{"delete", "postgres://x", "table", "extra"}, exit.Usage},
		{"no where or id", []string{"delete", "postgres://x", "table"}, exit.Usage},
		{
			"where and id are mutually exclusive",
			[]string{"delete", "postgres://x", "table", "--where", "id=1", "--id", "2"},
			exit.Usage,
		},
		{"plan not implemented", []string{"delete", "postgres://x", "table", "--id", "1", "--plan"}, exit.NotImplemented},
		{"unsupported dsn scheme", []string{"delete", "bogus://x", "table", "--id", "1"}, exit.Error},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			got := cli.Run(tc.args, &out, &errBuf)
			if got != tc.want {
				t.Errorf("exit = %d; want %d (stderr=%s)", got, tc.want, errBuf.String())
			}
		})
	}
}

func TestDeleteUsage_HelpFlag(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	got := cli.Run([]string{"delete", "--help"}, &out, &errBuf)
	if got != exit.OK {
		t.Fatalf("exit = %d; want %d", got, exit.OK)
	}
	if out.Len() == 0 {
		t.Error("expected usage text on stdout")
	}
}

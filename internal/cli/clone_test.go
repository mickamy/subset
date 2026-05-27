package cli_test

import (
	"bytes"
	"testing"

	"github.com/mickamy/subset/internal/cli"
	"github.com/mickamy/subset/internal/exit"
)

// TestRunClone_ArgErrors covers clone's argument-validation and early-failure
// paths that never reach the database, mirroring the delete coverage.
func TestRunClone_ArgErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing dsn and table", []string{"clone"}, exit.Usage},
		{"too many positionals", []string{"clone", "postgres://x", "table", "extra"}, exit.Usage},
		{"no where or id", []string{"clone", "postgres://x", "table"}, exit.Usage},
		{
			"where and id are mutually exclusive",
			[]string{"clone", "postgres://x", "table", "--where", "id=1", "--id", "2"},
			exit.Usage,
		},
		{"unsupported dsn scheme", []string{"clone", "bogus://x", "table", "--id", "1"}, exit.Error},
		{"unsupported dsn scheme with plan", []string{"clone", "bogus://x", "table", "--id", "1", "--plan"}, exit.Error},
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

func TestCloneUsage_HelpFlag(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	got := cli.Run([]string{"clone", "--help"}, &out, &errBuf)
	if got != exit.OK {
		t.Fatalf("exit = %d; want %d", got, exit.OK)
	}
	if out.Len() == 0 {
		t.Error("expected usage text on stdout")
	}
}

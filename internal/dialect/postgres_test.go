package dialect_test

import (
	"testing"

	"github.com/mickamy/subset/internal/dialect"
)

func TestPostgres_QuoteIdent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"users", `"users"`},
		{"snake_case_name", `"snake_case_name"`},
		{`with"quote`, `"with""quote"`},
		{"", `""`},
	}
	d := dialect.Postgres{}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := d.QuoteIdent(tc.in); got != tc.want {
				t.Errorf("QuoteIdent(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPostgres_Placeholder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		n    int
		want string
	}{
		{1, "$1"},
		{2, "$2"},
		{10, "$10"},
	}
	d := dialect.Postgres{}
	for _, tc := range cases {
		if got := d.Placeholder(tc.n); got != tc.want {
			t.Errorf("Placeholder(%d) = %q; want %q", tc.n, got, tc.want)
		}
	}
}

package dialect_test

import (
	"testing"

	"github.com/mickamy/subset/internal/dialect"
)

func TestMySQL_QuoteIdent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"users", "`users`"},
		{"snake_case_name", "`snake_case_name`"},
		{"with`backtick", "`with``backtick`"},
		{"", "``"},
	}
	d := dialect.MySQL{}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := d.QuoteIdent(tc.in); got != tc.want {
				t.Errorf("QuoteIdent(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMySQL_Placeholder(t *testing.T) {
	t.Parallel()

	d := dialect.MySQL{}
	for _, n := range []int{1, 2, 10} {
		if got := d.Placeholder(n); got != "?" {
			t.Errorf("Placeholder(%d) = %q; want \"?\"", n, got)
		}
	}
}

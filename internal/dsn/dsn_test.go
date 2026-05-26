package dsn_test

import (
	"strings"
	"testing"

	"github.com/mickamy/subset/internal/dsn"
)

func TestScheme(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"postgres://localhost/db", "postgres"},
		{"postgresql://localhost/db", "postgresql"},
		{"mysql://localhost:3306/db", "mysql"},
		{"no-scheme", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := dsn.Scheme(tc.in); got != tc.want {
			t.Errorf("Scheme(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestToMySQLDSN_InvalidInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"wrong scheme", "postgres://host/db", "expected mysql://"},
		{"missing host", "mysql:///db", "missing host"},
		{"missing db (no path)", "mysql://host:3306", "missing database name"},
		{"missing db (root path)", "mysql://host:3306/", "missing database name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := dsn.ToMySQLDSN(tc.in)
			if err == nil {
				t.Fatalf("ToMySQLDSN(%q): nil error, want failure", tc.in)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ToMySQLDSN(%q) error = %q; want substring %q", tc.in, err.Error(), tc.want)
			}
		})
	}
}

//nolint:gosec // G101: URL fixtures use placeholder credentials, not real
func TestToMySQLDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "full user/pass/host/port/db",
			in:   "mysql://root:pass@localhost:3306/dev",
			want: "root:pass@tcp(localhost:3306)/dev",
		},
		{
			name: "no password",
			in:   "mysql://user@host:3306/db",
			want: "user@tcp(host:3306)/db",
		},
		{
			name: "no user info",
			in:   "mysql://localhost:3306/dev",
			want: "tcp(localhost:3306)/dev",
		},
		{
			name: "with query params",
			in:   "mysql://root:pass@localhost:3306/dev?parseTime=true",
			want: "root:pass@tcp(localhost:3306)/dev?parseTime=true",
		},
		{
			name: "URL-encoded special char in password",
			in:   "mysql://root:p%23ss@host:3306/db",
			want: "root:p#ss@tcp(host:3306)/db",
		},
		{
			name: "URL-encoded @ in password is passed through as last-@ split applies",
			in:   "mysql://root:p%40ss@host:3306/db",
			want: "root:p@ss@tcp(host:3306)/db",
		},
		{
			name: "URL-encoded @ in username",
			in:   "mysql://us%40dom:pass@host:3306/db",
			want: "us@dom:pass@tcp(host:3306)/db",
		},
		{
			name: "URL-encoded @ in both username and password",
			in:   "mysql://us%40dom:pa%40ss@host:3306/db",
			want: "us@dom:pa@ss@tcp(host:3306)/db",
		},
		{
			// `:` in the username decodes to a literal colon, which the driver
			// then mistakes for the user/pass separator. Document this as a
			// known limitation; callers must avoid literal `:` in usernames.
			name: "URL-encoded : in username passes through verbatim (driver split is ambiguous)",
			in:   "mysql://us%3Aer:pass@host:3306/db",
			want: "us:er:pass@tcp(host:3306)/db",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dsn.ToMySQLDSN(tc.in)
			if err != nil {
				t.Fatalf("ToMySQLDSN(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ToMySQLDSN(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

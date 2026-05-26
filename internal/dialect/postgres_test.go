package dialect_test

import (
	"testing"
	"time"

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

func TestPostgres_QuoteLiteral(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 5, 26, 15, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "NULL"},
		{"true", true, "TRUE"},
		{"false", false, "FALSE"},
		{"empty string", "", "''"},
		{"simple string", "hello", "'hello'"},
		{"string with single quote", "it's", "'it''s'"},
		{"string with backslash is literal", `c:\path`, `'c:\path'`},
		{"empty bytes", []byte{}, `'\x'`},
		{"hex bytes", []byte{0xde, 0xad, 0xbe, 0xef}, `'\xdeadbeef'`},
		{"int positive", 42, "42"},
		{"int negative", -7, "-7"},
		{"int8", int8(127), "127"},
		{"int16", int16(-32000), "-32000"},
		{"int32", int32(123456), "123456"},
		{"int64 max", int64(9223372036854775807), "9223372036854775807"},
		{"uint", uint(42), "42"},
		{"uint8 (byte)", uint8(255), "255"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float64", 3.14, "3.14"},
		{"float64 zero", 0.0, "0"},
		{"float64 negative", -1.5, "-1.5"},
		{"float32", float32(2.5), "2.5"},
		{"time UTC", fixedTime, "'2026-05-26T15:30:00Z'"},
		{"unknown type fallback", struct{ N string }{"foo"}, "'{foo}'"},
	}

	d := dialect.Postgres{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := d.QuoteLiteral(tc.in); got != tc.want {
				t.Errorf("QuoteLiteral(%v) = %q; want %q", tc.in, got, tc.want)
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

package dialect_test

import (
	"testing"
	"time"

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

func TestMySQL_QuoteLiteral(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 5, 26, 15, 30, 0, 0, time.UTC)
	fixedTimeWithNanos := time.Date(2026, 5, 26, 15, 30, 0, 123456000, time.UTC)

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
		{"string with backslash is escaped", `c:\path`, `'c:\\path'`},
		{"empty bytes (valid UTF-8)", []byte{}, "''"},
		{"ASCII bytes are quoted as string", []byte("hello"), "'hello'"},
		{"decimal-like bytes (driver returns DECIMAL as []byte)", []byte("12.34"), "'12.34'"},
		{"JSON-like bytes", []byte(`{"x":1}`), `'{"x":1}'`},
		{"non-UTF-8 bytes fall back to hex", []byte{0xff, 0xfe, 0xfd}, "X'fffefd'"},
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
		{"time UTC", fixedTime, "'2026-05-26 15:30:00'"},
		{"time with microseconds", fixedTimeWithNanos, "'2026-05-26 15:30:00.123456'"},
		{"unknown type fallback", struct{ N string }{"foo"}, "'{foo}'"},
	}

	d := dialect.MySQL{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := d.QuoteLiteral(tc.in); got != tc.want {
				t.Errorf("QuoteLiteral(%v) = %q; want %q", tc.in, got, tc.want)
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

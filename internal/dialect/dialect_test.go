package dialect_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/dsn"
)

func TestNew(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want dialect.Dialect
	}{
		{"postgres scheme", "postgres://host/db", dialect.Postgres{}},
		{"postgresql scheme", "postgresql://host/db", dialect.Postgres{}},
		{"mysql scheme", "mysql://host/db", dialect.MySQL{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dialect.New(tc.in)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("New(%q) = %T; want %T", tc.in, got, tc.want)
			}
		})
	}
}

func TestNew_MissingScheme(t *testing.T) {
	t.Parallel()

	_, err := dialect.New("")
	if !errors.Is(err, dsn.ErrMissingScheme) {
		t.Errorf("err = %v; want dsn.ErrMissingScheme", err)
	}
}

func TestNew_UnsupportedScheme(t *testing.T) {
	t.Parallel()

	_, err := dialect.New("sqlite://file.db")
	if err == nil {
		t.Fatal("New returned nil error; want unsupported scheme error")
	}
	if !strings.Contains(err.Error(), "unsupported DSN scheme") {
		t.Errorf("err = %v; want unsupported scheme message", err)
	}
}

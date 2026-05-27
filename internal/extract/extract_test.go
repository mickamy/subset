package extract_test

import (
	"context"
	"testing"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/extract"
	"github.com/mickamy/subset/internal/introspect"
)

// TestWalk_ValidationErrors covers the checks Walk performs before it issues
// any query, so they run without a database connection (db is nil).
func TestWalk_ValidationErrors(t *testing.T) {
	t.Parallel()

	schema := introspect.Schema{Tables: []introspect.Table{
		{
			Name:       "users",
			PrimaryKey: []string{"id"},
			Columns:    []introspect.Column{{Name: "id"}},
		},
		{
			Name:    "no_pk",
			Columns: []introspect.Column{{Name: "x"}},
		},
	}}

	cases := []struct {
		name  string
		table string
		dir   extract.Direction
	}{
		{"unknown table forward", "ghost", extract.Forward},
		{"unknown table backward", "ghost", extract.Backward},
		{"seed without primary key forward", "no_pk", extract.Forward},
		{"seed without primary key backward", "no_pk", extract.Backward},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := extract.Walk(context.Background(), nil, dialect.Postgres{}, schema, tc.table, "", tc.dir)
			if err == nil {
				t.Fatalf("Walk(%q) = nil error; want error", tc.table)
			}
		})
	}
}

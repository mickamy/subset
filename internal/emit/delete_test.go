package emit_test

import (
	"testing"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/emit"
	"github.com/mickamy/subset/internal/introspect"
)

func TestBuildDelete_Postgres(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "email"},
		},
	}
	row := map[string]any{
		"id":    1,
		"email": "alice@example.com",
	}
	got := emit.BuildDelete(dialect.Postgres{}, table, row)
	want := `DELETE FROM "users" WHERE "id" = 1;`
	if got != want {
		t.Errorf("BuildDelete =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildDelete_Postgres_CompositePK(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "pages",
		PrimaryKey: []string{"site_id", "slug"},
		Columns: []introspect.Column{
			{Name: "site_id"},
			{Name: "slug"},
			{Name: "title"},
		},
	}
	row := map[string]any{
		"site_id": 1,
		"slug":    "home",
		"title":   "Home",
	}
	got := emit.BuildDelete(dialect.Postgres{}, table, row)
	want := `DELETE FROM "pages" WHERE "site_id" = 1 AND "slug" = 'home';`
	if got != want {
		t.Errorf("BuildDelete =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildDelete_Postgres_QuotesEmbeddedSingleQuote(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "users",
		PrimaryKey: []string{"name"},
		Columns:    []introspect.Column{{Name: "name"}},
	}
	row := map[string]any{"name": "O'Brien"}
	got := emit.BuildDelete(dialect.Postgres{}, table, row)
	want := `DELETE FROM "users" WHERE "name" = 'O''Brien';`
	if got != want {
		t.Errorf("BuildDelete =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildDelete_MySQL(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "email"},
		},
	}
	row := map[string]any{
		"id":    1,
		"email": "alice@example.com",
	}
	got := emit.BuildDelete(dialect.MySQL{}, table, row)
	want := "DELETE FROM `users` WHERE `id` = 1;"
	if got != want {
		t.Errorf("BuildDelete =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildDelete_MySQL_CompositePK(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "pages",
		PrimaryKey: []string{"site_id", "slug"},
		Columns: []introspect.Column{
			{Name: "site_id"},
			{Name: "slug"},
		},
	}
	row := map[string]any{
		"site_id": 1,
		"slug":    "home",
	}
	got := emit.BuildDelete(dialect.MySQL{}, table, row)
	want := "DELETE FROM `pages` WHERE `site_id` = 1 AND `slug` = 'home';"
	if got != want {
		t.Errorf("BuildDelete =\n  %s\nwant\n  %s", got, want)
	}
}

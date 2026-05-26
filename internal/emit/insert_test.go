package emit_test

import (
	"testing"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/emit"
	"github.com/mickamy/subset/internal/introspect"
)

func TestBuildInsert_Postgres(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "email"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":    1,
		"email": "alice@example.com",
		"name":  "Alice",
	}
	got := emit.BuildInsert(dialect.Postgres{}, table, row)
	want := `INSERT INTO "users" ("id", "email", "name") VALUES (1, 'alice@example.com', 'Alice');`
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_Postgres_WithNULL(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":   1,
		"name": nil,
	}
	got := emit.BuildInsert(dialect.Postgres{}, table, row)
	want := `INSERT INTO "users" ("id", "name") VALUES (1, NULL);`
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_Postgres_SkipsGeneratedColumn(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "products",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "cost_yen"},
			{Name: "margin_yen"},
			{Name: "price_yen", IsGenerated: true},
		},
	}
	row := map[string]any{
		"id":         1,
		"cost_yen":   100,
		"margin_yen": 50,
		"price_yen":  150,
	}
	got := emit.BuildInsert(dialect.Postgres{}, table, row)
	want := `INSERT INTO "products" ("id", "cost_yen", "margin_yen") VALUES (1, 100, 50);`
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_Postgres_MissingKeyIsNULL(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id": 1,
		// name is missing
	}
	got := emit.BuildInsert(dialect.Postgres{}, table, row)
	want := `INSERT INTO "users" ("id", "name") VALUES (1, NULL);`
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_Postgres_QuotesEmbeddedSingleQuote(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":   1,
		"name": "O'Brien",
	}
	got := emit.BuildInsert(dialect.Postgres{}, table, row)
	want := `INSERT INTO "users" ("id", "name") VALUES (1, 'O''Brien');`
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_MySQL(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "email"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":    1,
		"email": "alice@example.com",
		"name":  "Alice",
	}
	got := emit.BuildInsert(dialect.MySQL{}, table, row)
	want := "INSERT INTO `users` (`id`, `email`, `name`) VALUES (1, 'alice@example.com', 'Alice');"
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_MySQL_WithNULL(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":   1,
		"name": nil,
	}
	got := emit.BuildInsert(dialect.MySQL{}, table, row)
	want := "INSERT INTO `users` (`id`, `name`) VALUES (1, NULL);"
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

func TestBuildInsert_MySQL_EscapesBackslashAndQuote(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	row := map[string]any{
		"id":   1,
		"name": `O'Brien\test`,
	}
	got := emit.BuildInsert(dialect.MySQL{}, table, row)
	want := "INSERT INTO `users` (`id`, `name`) VALUES (1, 'O''Brien\\\\test');"
	if got != want {
		t.Errorf("BuildInsert =\n  %s\nwant\n  %s", got, want)
	}
}

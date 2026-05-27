package extract

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/introspect"
)

// Row maps column names to scanned values for one extracted row.
type Row = map[string]any

// Collected groups extracted rows by table name. Ordering for emission
// is the caller's responsibility (see internal/plan.Build).
type Collected struct {
	Rows map[string][]Row
}

// Walk extracts rows from a seed (seedTable + whereClause) and recursively
// collects FK-referenced parent rows (forward closure). Used by clone.
//
// An empty whereClause selects every row in seedTable (no WHERE filter
// is appended). The CLI rejects empty filters; this is for callers that
// want to clone the full contents of a small lookup table.
//
// Composite primary keys, composite foreign keys, and self-referencing
// FKs are all supported. For self-ref tables, callers should run the
// per-table row slice through SortByPK before emitting so that referenced
// rows appear before their references.
func Walk(
	ctx context.Context,
	db *sql.DB,
	d dialect.Dialect,
	schema introspect.Schema,
	seedTable, whereClause string,
) (*Collected, error) {
	tableByName := make(map[string]introspect.Table, len(schema.Tables))
	for _, t := range schema.Tables {
		tableByName[t.Name] = t
	}
	seed, ok := tableByName[seedTable]
	if !ok {
		return nil, fmt.Errorf("unknown table %q", seedTable)
	}
	if len(seed.PrimaryKey) == 0 {
		return nil, fmt.Errorf("table %q has no primary key", seed.Name)
	}

	collected := &Collected{Rows: make(map[string][]Row)}
	visited := make(map[string]map[string]bool)

	seedQuery := selectAll(d, seed)
	if whereClause != "" {
		seedQuery += " WHERE " + whereClause
	}
	seedRows, err := querySelect(ctx, db, seedQuery, nil, seed)
	if err != nil {
		return nil, fmt.Errorf("seed select: %w", err)
	}

	type queueItem struct {
		table introspect.Table
		row   Row
	}
	queue := make([]queueItem, 0, len(seedRows))
	for _, row := range seedRows {
		if markIfNew(visited, seed.Name, compositeKey(row, seed.PrimaryKey)) {
			collected.Rows[seed.Name] = append(collected.Rows[seed.Name], row)
			queue = append(queue, queueItem{seed, row})
		}
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		for _, fk := range item.table.ForeignKeys {
			parent, ok := tableByName[fk.ReferencedTable]
			if !ok {
				continue
			}
			if len(parent.PrimaryKey) == 0 {
				return nil, fmt.Errorf("table %q has no primary key", parent.Name)
			}

			args, ok := fkArgs(item.row, fk.Columns)
			if !ok {
				continue
			}
			parentQuery := fmt.Sprintf("%s WHERE %s",
				selectAll(d, parent),
				fkWhereClause(d, fk.ReferencedColumns))
			parentRows, err := querySelect(ctx, db, parentQuery, args, parent)
			if err != nil {
				return nil, fmt.Errorf("fetch parent %q: %w", parent.Name, err)
			}

			for _, prow := range parentRows {
				if markIfNew(visited, parent.Name, compositeKey(prow, parent.PrimaryKey)) {
					collected.Rows[parent.Name] = append(collected.Rows[parent.Name], prow)
					queue = append(queue, queueItem{parent, prow})
				}
			}
		}
	}

	return collected, nil
}

func selectAll(d dialect.Dialect, table introspect.Table) string {
	cols := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		cols[i] = d.QuoteIdent(c.Name)
	}

	return fmt.Sprintf("SELECT %s FROM %s",
		strings.Join(cols, ", "), d.QuoteIdent(table.Name))
}

// fkArgs returns FK column values from a row in cols order. If any value
// is NULL, returns (nil, false): a composite FK with any NULL component
// references no row (SQL standard).
func fkArgs(row Row, cols []string) ([]any, bool) {
	args := make([]any, len(cols))
	for i, col := range cols {
		v := row[col]
		if v == nil {
			return nil, false
		}
		args[i] = v
	}

	return args, true
}

// fkWhereClause builds a conjunction matching cols against placeholders
// $1..$N (Postgres) or ?..? (MySQL).
func fkWhereClause(d dialect.Dialect, cols []string) string {
	clauses := make([]string, len(cols))
	for i, col := range cols {
		clauses[i] = fmt.Sprintf("%s = %s",
			d.QuoteIdent(col), d.Placeholder(i+1))
	}

	return strings.Join(clauses, " AND ")
}

// compositeKey returns a stable string representation of a row's values at
// the named columns, used as the visited-set key. The `%#v` format embeds
// types in the output, so int(12) and string("12") produce distinct keys.
func compositeKey(row Row, cols []string) string {
	vals := make([]any, len(cols))
	for i, col := range cols {
		vals[i] = row[col]
	}

	return fmt.Sprintf("%#v", vals)
}

// querySelect runs a SELECT and returns rows as column-name maps. MySQL's
// driver returns []byte for text columns; introspect.Kind tells us when
// to coerce that back to string while preserving real bytea/BLOB.
func querySelect(ctx context.Context, db *sql.DB, query string, args []any, table introspect.Table) ([]Row, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	colKinds := make(map[string]introspect.Kind, len(table.Columns))
	for _, c := range table.Columns {
		colKinds[c.Name] = c.Kind
	}

	var result []Row
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		row := make(Row, len(cols))
		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok && colKinds[col] != introspect.KindBytes {
				v = string(b)
			}
			row[col] = v
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return result, nil
}

func markIfNew(visited map[string]map[string]bool, table, pk string) bool {
	if visited[table] == nil {
		visited[table] = make(map[string]bool)
	}
	if visited[table][pk] {
		return false
	}
	visited[table][pk] = true

	return true
}

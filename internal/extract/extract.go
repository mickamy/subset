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

// Direction selects which way Walk traverses the FK graph from the seed.
type Direction int

const (
	// Forward collects the FK parents a row references (clone). Emit the
	// result parents-first so every INSERT lands after its referenced rows.
	Forward Direction = iota
	// Backward collects the rows that reference a row (delete). Emit the
	// result most-dependent-first so every DELETE runs before the row it
	// depends on.
	Backward
)

// Walk extracts rows from a seed (seedTable + whereClause) and recursively
// collects the FK-connected closure in the given direction: parents for
// Forward (clone), referencing children for Backward (delete).
//
// An empty whereClause selects every row in seedTable (no WHERE filter
// is appended). The CLI rejects empty filters; this is for callers that
// want to operate on the full contents of a small lookup table.
//
// Composite primary keys, composite foreign keys, and self-referencing
// FKs are all supported. Within a single table, callers order the row slice
// with SortByPK: emit it as-is for Forward (referenced rows first), reversed
// for Backward (referencing rows first).
func Walk(
	ctx context.Context,
	db *sql.DB,
	d dialect.Dialect,
	schema introspect.Schema,
	seedTable, whereClause string,
	dir Direction,
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

	// For Backward, index FKs by the table they reference so we can find the
	// rows pointing at a given row. Forward reads FKs straight off the row's
	// own table, so it needs no index.
	var referencers map[string][]referencer
	if dir == Backward {
		referencers = buildReferencers(schema)
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

		var groups []fetchedGroup
		switch dir {
		case Forward:
			groups, err = walkForward(ctx, db, d, item, tableByName)
		case Backward:
			groups, err = walkBackward(ctx, db, d, item, referencers[item.table.Name])
		}
		if err != nil {
			return nil, err
		}

		for _, g := range groups {
			for _, row := range g.rows {
				if markIfNew(visited, g.table.Name, compositeKey(row, g.table.PrimaryKey)) {
					collected.Rows[g.table.Name] = append(collected.Rows[g.table.Name], row)
					queue = append(queue, queueItem{g.table, row})
				}
			}
		}
	}

	return collected, nil
}

type queueItem struct {
	table introspect.Table
	row   Row
}

// fetchedGroup pairs a target table with the rows fetched for it during one
// expansion step, ready to be deduped and enqueued by Walk.
type fetchedGroup struct {
	table introspect.Table
	rows  []Row
}

// referencer is a foreign key together with the table that declares it, used
// to walk backward from a referenced row to the rows that reference it.
type referencer struct {
	table introspect.Table
	fk    introspect.ForeignKey
}

// buildReferencers indexes every FK by the table it references, so a Backward
// walk can find all rows pointing at a given table (self-references included).
func buildReferencers(schema introspect.Schema) map[string][]referencer {
	index := make(map[string][]referencer)
	for _, t := range schema.Tables {
		for _, fk := range t.ForeignKeys {
			index[fk.ReferencedTable] = append(index[fk.ReferencedTable], referencer{t, fk})
		}
	}

	return index
}

// walkForward fetches the FK parents referenced by item.row: for each FK on
// item's table, select the parent row matching the FK column values.
func walkForward(
	ctx context.Context,
	db *sql.DB,
	d dialect.Dialect,
	item queueItem,
	tableByName map[string]introspect.Table,
) ([]fetchedGroup, error) {
	var groups []fetchedGroup
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
		query := fmt.Sprintf("%s WHERE %s",
			selectAll(d, parent),
			fkWhereClause(d, fk.ReferencedColumns))
		rows, err := querySelect(ctx, db, query, args, parent)
		if err != nil {
			return nil, fmt.Errorf("fetch parent %q: %w", parent.Name, err)
		}
		groups = append(groups, fetchedGroup{parent, rows})
	}

	return groups, nil
}

// walkBackward fetches the rows that reference item.row: for each FK pointing
// at item's table, select the child rows whose FK columns match item.row's
// referenced values.
func walkBackward(
	ctx context.Context,
	db *sql.DB,
	d dialect.Dialect,
	item queueItem,
	refs []referencer,
) ([]fetchedGroup, error) {
	var groups []fetchedGroup
	for _, ref := range refs {
		child := ref.table
		if len(child.PrimaryKey) == 0 {
			return nil, fmt.Errorf("table %q has no primary key", child.Name)
		}

		args, ok := fkArgs(item.row, ref.fk.ReferencedColumns)
		if !ok {
			continue
		}
		query := fmt.Sprintf("%s WHERE %s",
			selectAll(d, child),
			fkWhereClause(d, ref.fk.Columns))
		rows, err := querySelect(ctx, db, query, args, child)
		if err != nil {
			return nil, fmt.Errorf("fetch child %q: %w", child.Name, err)
		}
		groups = append(groups, fetchedGroup{child, rows})
	}

	return groups, nil
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

package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/mickamy/subset/internal/dsn"
)

type mySQLDriver struct {
	db *sql.DB
}

func newMySQLDriver(ctx context.Context, dataSourceName string) (*mySQLDriver, error) {
	converted, err := dsn.ToMySQLDSN(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("dsn: %w", err)
	}
	db, err := sql.Open("mysql", converted)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("ping: %w", err)
	}

	return &mySQLDriver{db: db}, nil
}

func (d *mySQLDriver) Close(_ context.Context) error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

func (d *mySQLDriver) Introspect(ctx context.Context) (Schema, error) {
	tables, err := d.fetchTablesWithColumns(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("fetch tables: %w", err)
	}
	if err := d.fetchPrimaryKeys(ctx, tables); err != nil {
		return Schema{}, fmt.Errorf("fetch primary keys: %w", err)
	}
	if err := d.fetchForeignKeys(ctx, tables); err != nil {
		return Schema{}, fmt.Errorf("fetch foreign keys: %w", err)
	}
	uniques, err := d.fetchUniques(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("fetch uniques: %w", err)
	}

	names := slices.Sorted(maps.Keys(tables))
	out := make([]Table, 0, len(names))
	for _, n := range names {
		t := tables[n]
		pkIsSingle := len(t.PrimaryKey) == 1
		for i := range t.Columns {
			c := &t.Columns[i]
			if uniques.single[t.Name][c.Name] {
				c.IsUnique = true
			}
			if pkIsSingle && c.Name == t.PrimaryKey[0] {
				c.IsUnique = true
			}
		}
		t.CompositeUniques = uniques.composite[t.Name]
		out = append(out, *t)
	}

	return Schema{Tables: out}, nil
}

const mySQLTablesQuery = `
SELECT
    c.table_name,
    c.column_name,
    c.data_type,
    c.column_type,
    c.is_nullable,
    c.column_default,
    c.extra,
    c.character_maximum_length
FROM information_schema.tables t
JOIN information_schema.columns c
  ON c.table_schema = t.table_schema
 AND c.table_name   = t.table_name
WHERE t.table_schema = DATABASE()
  AND t.table_type  = 'BASE TABLE'
ORDER BY c.table_name, c.ordinal_position
`

func (d *mySQLDriver) fetchTablesWithColumns(ctx context.Context) (map[string]*Table, error) {
	rows, err := d.db.QueryContext(ctx, mySQLTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tables := make(map[string]*Table)
	for rows.Next() {
		var (
			tname, cname, dataType, columnType string
			isNullable, extra                  string
			colDefault                         sql.NullString
			maxLen                             sql.NullInt64
		)
		if err := rows.Scan(&tname, &cname, &dataType, &columnType, &isNullable, &colDefault, &extra, &maxLen); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		t, ok := tables[tname]
		if !ok {
			t = &Table{Name: tname}
			tables[tname] = t
		}
		kind, enumValues := mySQLKind(dataType, columnType)
		var defaultVal *string
		if colDefault.Valid {
			s := colDefault.String
			defaultVal = &s
		}
		var maxLength int
		if maxLen.Valid {
			maxLength = int(maxLen.Int64)
		}
		extraLower := strings.ToLower(extra)
		// VIRTUAL / STORED GENERATED match true generated columns; DEFAULT_GENERATED
		// in the same field marks expression defaults (e.g., CURRENT_TIMESTAMP)
		// on otherwise-writable columns, so it must not be treated as generated.
		isGenerated := strings.Contains(extraLower, "virtual generated") ||
			strings.Contains(extraLower, "stored generated")
		t.Columns = append(t.Columns, Column{
			Name:        cname,
			DataType:    dataType,
			EnumValues:  enumValues,
			Kind:        kind,
			Nullable:    isNullable == "YES",
			Default:     defaultVal,
			IsIdentity:  strings.Contains(extraLower, "auto_increment"),
			IsGenerated: isGenerated,
			MaxLength:   maxLength,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return tables, nil
}

const mySQLPrimaryKeysQuery = `
SELECT k.table_name, k.column_name
FROM information_schema.table_constraints t
JOIN information_schema.key_column_usage k
  ON t.constraint_name = k.constraint_name
 AND t.table_schema    = k.table_schema
 AND t.table_name      = k.table_name
WHERE t.constraint_type = 'PRIMARY KEY'
  AND t.table_schema    = DATABASE()
ORDER BY k.table_name, k.ordinal_position
`

func (d *mySQLDriver) fetchPrimaryKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.db.QueryContext(ctx, mySQLPrimaryKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tname, cname string
		if err := rows.Scan(&tname, &cname); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if t, ok := tables[tname]; ok {
			t.PrimaryKey = append(t.PrimaryKey, cname)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	return nil
}

const mySQLForeignKeysQuery = `
SELECT
    k.constraint_name,
    k.table_name,
    k.column_name,
    k.referenced_table_name,
    k.referenced_column_name
FROM information_schema.key_column_usage k
WHERE k.table_schema          = DATABASE()
  AND k.referenced_table_name IS NOT NULL
ORDER BY k.table_name, k.constraint_name, k.ordinal_position
`

func (d *mySQLDriver) fetchForeignKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.db.QueryContext(ctx, mySQLForeignKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type fkKey struct{ table, name string }
	fks := make(map[fkKey]*ForeignKey)
	order := make([]fkKey, 0)
	for rows.Next() {
		var conName, tname, cname, fTable, fCol string
		if err := rows.Scan(&conName, &tname, &cname, &fTable, &fCol); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		key := fkKey{table: tname, name: conName}
		fk, ok := fks[key]
		if !ok {
			fk = &ForeignKey{Name: conName, ReferencedTable: fTable}
			fks[key] = fk
			order = append(order, key)
		}
		fk.Columns = append(fk.Columns, cname)
		fk.ReferencedColumns = append(fk.ReferencedColumns, fCol)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	for _, k := range order {
		if t, ok := tables[k.table]; ok {
			t.ForeignKeys = append(t.ForeignKeys, *fks[k])
		}
	}

	return nil
}

// Returns every UNIQUE column; fetchUniques routes single-column constraints
// to Column.IsUnique and keeps composite ones as Table.CompositeUniques.
const mySQLUniquesQuery = `
SELECT
    tc.constraint_name,
    kcu.table_name,
    kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema    = kcu.table_schema
 AND tc.table_name      = kcu.table_name
WHERE tc.constraint_type = 'UNIQUE'
  AND tc.table_schema    = DATABASE()
ORDER BY kcu.table_name, tc.constraint_name, kcu.ordinal_position
`

func (d *mySQLDriver) fetchUniques(ctx context.Context) (uniquesInfo, error) {
	rows, err := d.db.QueryContext(ctx, mySQLUniquesQuery)
	if err != nil {
		return uniquesInfo{}, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// MySQL constraint names are only unique per-table, so key by (table, name).
	type constraintKey struct{ table, name string }
	byConstraint := make(map[constraintKey][]string)
	for rows.Next() {
		var conName, tname, cname string
		if err := rows.Scan(&conName, &tname, &cname); err != nil {
			return uniquesInfo{}, fmt.Errorf("scan: %w", err)
		}
		k := constraintKey{table: tname, name: conName}
		byConstraint[k] = append(byConstraint[k], cname)
	}
	if err := rows.Err(); err != nil {
		return uniquesInfo{}, fmt.Errorf("rows: %w", err)
	}

	out := uniquesInfo{
		single:    make(map[string]map[string]bool),
		composite: make(map[string][][]string),
	}
	for k, cols := range byConstraint {
		if len(cols) == 1 {
			if out.single[k.table] == nil {
				out.single[k.table] = make(map[string]bool)
			}
			out.single[k.table][cols[0]] = true
			continue
		}
		if len(cols) > 1 {
			out.composite[k.table] = append(out.composite[k.table], cols)
		}
	}

	return out, nil
}

func mySQLKind(dataType, columnType string) (Kind, []string) {
	ct := strings.ToLower(strings.TrimSpace(columnType))
	switch strings.ToLower(dataType) {
	case "tinyint":
		// `tinyint(1)` is boolean by MySQL convention; modifiers such as
		// `unsigned` are tolerated (e.g., `tinyint(1) unsigned`). The
		// display-width `(1)` is deprecated in MySQL 8.0.17+; columns that
		// drop it fall through to KindInt here.
		if ct == "tinyint(1)" || strings.HasPrefix(ct, "tinyint(1) ") {
			return KindBool, nil
		}

		return KindInt, nil
	case "smallint", "mediumint", "int", "integer", "bigint":
		return KindInt, nil
	case "float", "double", "decimal", "numeric", "real":
		return KindFloat, nil
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		return KindString, nil
	case "date":
		return KindDate, nil
	case "datetime", "timestamp":
		return KindTimestamp, nil
	case "time", "year":
		return KindTime, nil
	case "json":
		return KindJSON, nil
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return KindBytes, nil
	case "enum", "set":
		return KindEnum, parseMySQLEnumLabels(columnType)
	case "bit":
		// `bit(1)` behaves like boolean; wider bit fields are byte-strings.
		if ct == "bit(1)" || strings.HasPrefix(ct, "bit(1) ") {
			return KindBool, nil
		}

		return KindBytes, nil
	}

	return KindUnknown, nil
}

// enumLabelRe matches each quoted label inside an `enum('a','b','c')`
// or `set('a','b','c')` MySQL column_type literal. MySQL encodes a label that itself
// contains a single quote as two consecutive ASCII apostrophes; parseMySQLEnumLabels
// collapses that pair back to one.
var enumLabelRe = regexp.MustCompile(`'((?:[^']|'')*)'`)

func parseMySQLEnumLabels(columnType string) []string {
	matches := enumLabelRe.FindAllStringSubmatch(columnType, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.ReplaceAll(m[1], "''", "'"))
	}

	return out
}

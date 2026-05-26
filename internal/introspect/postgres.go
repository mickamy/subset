package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
)

type postgresDriver struct {
	conn *pgx.Conn
}

func newPostgresDriver(ctx context.Context, dataSourceName string) (*postgresDriver, error) {
	conn, err := pgx.Connect(ctx, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return &postgresDriver{conn: conn}, nil
}

func (d *postgresDriver) Close(ctx context.Context) error {
	if err := d.conn.Close(ctx); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

func (d *postgresDriver) Introspect(ctx context.Context) (Schema, error) {
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

	enumLabels, err := d.fetchEnums(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("fetch enums: %w", err)
	}

	names := slices.Sorted(maps.Keys(tables))
	out := make([]Table, 0, len(names))
	for _, n := range names {
		t := tables[n]
		pkIsSingle := len(t.PrimaryKey) == 1
		for i := range t.Columns {
			c := &t.Columns[i]
			c.Kind = pgKind(c.DataType, c.UDTName, enumLabels)
			if c.Kind == KindEnum {
				c.EnumValues = enumLabels[c.UDTName]
			}
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

func pgKind(dataType, udtName string, enums map[string][]string) Kind {
	switch dataType {
	case "boolean":
		return KindBool
	case "smallint", "integer", "bigint":
		return KindInt
	case "real", "double precision", "numeric", "decimal":
		return KindFloat
	case "text", "character varying", "character", "name":
		return KindString
	case "uuid":
		return KindUUID
	case "date":
		return KindDate
	case "timestamp", "timestamp without time zone", "timestamp with time zone", "timestamptz":
		return KindTimestamp
	case "time", "time without time zone", "time with time zone", "timetz":
		return KindTime
	case "json", "jsonb":
		return KindJSON
	case "bytea":
		return KindBytes
	case "USER-DEFINED":
		if _, ok := enums[udtName]; ok {
			return KindEnum
		}

		return KindUnknown
	default:
		return KindUnknown
	}
}

// pgTablesQuery requires Postgres 12+: is_generated was introduced with the
// GENERATED ALWAYS AS ... STORED feature. Older servers will fail here with
// "column is_generated does not exist".
const pgTablesQuery = `
SELECT
    c.table_name,
    c.column_name,
    c.data_type,
    c.udt_name,
    c.is_nullable,
    c.column_default,
    c.is_identity,
    c.character_maximum_length,
    c.is_generated
FROM information_schema.tables t
JOIN information_schema.columns c
  ON c.table_schema = t.table_schema
 AND c.table_name   = t.table_name
WHERE t.table_schema = current_schema()
  AND t.table_type  = 'BASE TABLE'
ORDER BY c.table_name, c.ordinal_position
`

func (d *postgresDriver) fetchTablesWithColumns(ctx context.Context) (map[string]*Table, error) {
	rows, err := d.conn.Query(ctx, pgTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]*Table)
	for rows.Next() {
		var (
			tname, cname, dataType, udtName string
			isNullable, isIdentity          string
			colDefault                      sql.NullString
			maxLen                          sql.NullInt64
			isGenerated                     string
		)
		if err := rows.Scan(
			&tname, &cname, &dataType, &udtName,
			&isNullable, &colDefault, &isIdentity, &maxLen, &isGenerated,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		t, ok := tables[tname]
		if !ok {
			t = &Table{Name: tname}
			tables[tname] = t
		}
		var defaultVal *string
		if colDefault.Valid {
			s := colDefault.String
			defaultVal = &s
		}
		var maxLength int
		if maxLen.Valid {
			maxLength = int(maxLen.Int64)
		}
		t.Columns = append(t.Columns, Column{
			Name:        cname,
			DataType:    dataType,
			UDTName:     udtName,
			Nullable:    isNullable == "YES",
			Default:     defaultVal,
			IsIdentity:  isIdentity == "YES",
			IsGenerated: isGenerated == "ALWAYS",
			MaxLength:   maxLength,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return tables, nil
}

const pgPrimaryKeysQuery = `
SELECT kcu.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name   = kcu.constraint_name
 AND tc.constraint_schema = kcu.constraint_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema    = current_schema()
ORDER BY kcu.table_name, kcu.ordinal_position
`

func (d *postgresDriver) fetchPrimaryKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.conn.Query(ctx, pgPrimaryKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

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

// pg_constraint.conkey/confkey preserves composite FK column ordering
// (information_schema joins by constraint name and Cartesian-products it).
const pgForeignKeysQuery = `
SELECT
    con.conname,
    cl.relname  AS table_name,
    att.attname AS column_name,
    fcl.relname AS foreign_table_name,
    fatt.attname AS foreign_column_name
FROM pg_constraint con
JOIN pg_class      cl   ON cl.oid  = con.conrelid
JOIN pg_class      fcl  ON fcl.oid = con.confrelid
JOIN pg_namespace  ns   ON ns.oid  = cl.relnamespace
JOIN unnest(con.conkey, con.confkey) WITH ORDINALITY AS u(conkey, confkey, ord)
  ON TRUE
JOIN pg_attribute  att  ON att.attrelid  = con.conrelid  AND att.attnum  = u.conkey
JOIN pg_attribute  fatt ON fatt.attrelid = con.confrelid AND fatt.attnum = u.confkey
WHERE con.contype = 'f'
  AND ns.nspname  = current_schema()
ORDER BY cl.relname, con.conname, u.ord
`

func (d *postgresDriver) fetchForeignKeys(ctx context.Context, tables map[string]*Table) error {
	rows, err := d.conn.Query(ctx, pgForeignKeysQuery)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

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
const pgUniquesQuery = `
SELECT
    tc.constraint_name,
    kcu.table_name,
    kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name   = kcu.constraint_name
 AND tc.constraint_schema = kcu.constraint_schema
WHERE tc.constraint_type = 'UNIQUE'
  AND tc.table_schema    = current_schema()
ORDER BY kcu.table_name, tc.constraint_name, kcu.ordinal_position
`

// Postgres constraint names are unique within a schema, so the constraint_name
// alone is enough to group rows belonging to the same UNIQUE constraint.
func (d *postgresDriver) fetchUniques(ctx context.Context) (uniquesInfo, error) {
	rows, err := d.conn.Query(ctx, pgUniquesQuery)
	if err != nil {
		return uniquesInfo{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	type tableCol struct{ table, column string }
	byConstraint := make(map[string][]tableCol)
	for rows.Next() {
		var conName, tname, cname string
		if err := rows.Scan(&conName, &tname, &cname); err != nil {
			return uniquesInfo{}, fmt.Errorf("scan: %w", err)
		}
		byConstraint[conName] = append(byConstraint[conName], tableCol{tname, cname})
	}
	if err := rows.Err(); err != nil {
		return uniquesInfo{}, fmt.Errorf("rows: %w", err)
	}

	out := uniquesInfo{
		single:    make(map[string]map[string]bool),
		composite: make(map[string][][]string),
	}
	for _, cols := range byConstraint {
		if len(cols) == 0 {
			continue
		}
		table := cols[0].table
		if len(cols) == 1 {
			if out.single[table] == nil {
				out.single[table] = make(map[string]bool)
			}
			out.single[table][cols[0].column] = true
			continue
		}
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = c.column
		}
		out.composite[table] = append(out.composite[table], colNames)
	}

	return out, nil
}

const pgEnumsQuery = `
SELECT t.typname, e.enumlabel
FROM pg_type t
JOIN pg_enum e                 ON t.oid = e.enumtypid
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = current_schema()
ORDER BY t.typname, e.enumsortorder
`

func (d *postgresDriver) fetchEnums(ctx context.Context) (map[string][]string, error) {
	rows, err := d.conn.Query(ctx, pgEnumsQuery)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]string)
	for rows.Next() {
		var name, label string
		if err := rows.Scan(&name, &label); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out[name] = append(out[name], label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return out, nil
}

package extract

import (
	"fmt"

	"github.com/mickamy/subset/internal/introspect"
)

// SortByPK reorders rows so that any self-referencing FK is satisfied at
// apply time: a row whose FK column points to another row in the same
// slice will appear after its parent. Tables without a self-ref FK are
// returned unchanged.
//
// v0.1 supports single-column self-ref FKs against single-column PKs.
// Composite self-ref FKs or composite PKs return an error. Cyclic data
// (e.g., A -> B -> A) also returns an error: the apply step would need a
// DEFERRABLE constraint to succeed, which is out of scope.
func SortByPK(rows []Row, table introspect.Table) ([]Row, error) {
	selfRefs := findSelfRefs(table)
	if len(selfRefs) == 0 || len(rows) == 0 {
		return rows, nil
	}
	if len(table.PrimaryKey) != 1 {
		return nil, fmt.Errorf(
			"table %q needs a single-column primary key for self-ref ordering (has %d)",
			table.Name, len(table.PrimaryKey),
		)
	}
	refCols := make([]string, 0, len(selfRefs))
	for _, fk := range selfRefs {
		if len(fk.Columns) != 1 || len(fk.ReferencedColumns) != 1 {
			return nil, fmt.Errorf(
				"table %q has composite self-referencing FK %q; v0.1 supports single-column self-refs",
				table.Name, fk.Name)
		}
		refCols = append(refCols, fk.Columns[0])
	}

	pkCol := table.PrimaryKey[0]

	byPK := make(map[any]Row, len(rows))
	for _, row := range rows {
		pkVal := row[pkCol]
		if pkVal == nil {
			return nil, fmt.Errorf("table %q has a row with nil primary key %q", table.Name, pkCol)
		}
		byPK[normalizeKey(pkVal)] = row
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	state := make(map[any]int, len(rows))
	result := make([]Row, 0, len(rows))

	var visit func(row Row) error
	visit = func(row Row) error {
		rowKey := normalizeKey(row[pkCol])
		switch state[rowKey] {
		case black:
			return nil
		case gray:
			return fmt.Errorf(
				"table %q has cyclic data via self-referencing FK(s) starting at pk=%v",
				table.Name, row[pkCol])
		}
		state[rowKey] = gray

		// Visit the parent referenced by every self-ref FK, so the emitted
		// order satisfies all of them (e.g., both manager_id and mentor_id).
		for _, refCol := range refCols {
			refVal := row[refCol]
			if refVal == nil {
				continue
			}
			if parent, ok := byPK[normalizeKey(refVal)]; ok {
				if err := visit(parent); err != nil {
					return err
				}
			}
			// FK pointing outside the collected set is fine — that row is
			// either already in the target DB or out of scope.
		}

		state[rowKey] = black
		result = append(result, row)

		return nil
	}

	for _, row := range rows {
		if err := visit(row); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func findSelfRefs(table introspect.Table) []introspect.ForeignKey {
	var refs []introspect.ForeignKey
	for i := range table.ForeignKeys {
		if table.ForeignKeys[i].ReferencedTable == table.Name {
			refs = append(refs, table.ForeignKeys[i])
		}
	}

	return refs
}

// normalizeKey returns a comparable representation of v suitable as a Go
// map key. Integer widths are unified to int64 / uint64 so that drivers
// returning the same logical PK value with different concrete types
// (e.g., int vs int64 in tests) hash to the same bucket. []byte is
// converted to string because slices are not comparable.
func normalizeKey(v any) any {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case uint:
		return uint64(val)
	case uint8:
		return uint64(val)
	case uint16:
		return uint64(val)
	case uint32:
		return uint64(val)
	case uint64:
		return val
	case []byte:
		return string(val)
	default:
		return val
	}
}

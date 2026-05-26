package plan

import (
	"fmt"
	"strings"

	"github.com/mickamy/subset/internal/introspect"
)

// Build returns table names in dependency order: referenced (parent) tables
// come before referencing (child) tables. Self-referencing FKs are ignored
// for ordering. Returns an error if a cyclic FK dependency (length >= 2)
// is detected.
func Build(tables []introspect.Table) ([]string, error) {
	names := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		names[t.Name] = struct{}{}
	}

	deps := make(map[string][]string, len(tables))
	for _, t := range tables {
		seen := make(map[string]struct{})
		for _, fk := range t.ForeignKeys {
			if fk.ReferencedTable == t.Name {
				continue
			}
			if _, ok := names[fk.ReferencedTable]; !ok {
				continue
			}
			if _, dup := seen[fk.ReferencedTable]; dup {
				continue
			}
			seen[fk.ReferencedTable] = struct{}{}
			deps[t.Name] = append(deps[t.Name], fk.ReferencedTable)
		}
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)

	order := make([]string, 0, len(tables))
	state := make(map[string]int, len(tables))

	var visit func(string, []string) error
	visit = func(n string, stack []string) error {
		switch state[n] {
		case visiting:
			path := append(stack, n) //nolint:gocritic // intentional: path is local to the error message
			return fmt.Errorf("cyclic FK dependency: %s", strings.Join(path, " -> "))
		case done:
			return nil
		}
		state[n] = visiting
		for _, dep := range deps[n] {
			if err := visit(dep, append(stack, n)); err != nil {
				return err
			}
		}
		state[n] = done
		order = append(order, n)

		return nil
	}

	for _, t := range tables {
		if err := visit(t.Name, nil); err != nil {
			return nil, err
		}
	}

	return order, nil
}

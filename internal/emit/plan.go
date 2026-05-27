package emit

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/mickamy/subset/internal/introspect"
)

// WritePlan renders the --plan summary: a header with table/row totals and one
// line per collected table, each showing its row count and a representative
// shortest FK path from the seed. cmd is "clone" or "delete"; forward selects
// the traversal direction (clone walks to FK parents, delete to referencing
// children). order lists the collected tables in emit order; counts holds each
// table's row count.
func WritePlan(
	w io.Writer,
	cmd string,
	forward bool,
	schema introspect.Schema,
	counts map[string]int,
	order []string,
	seed string,
	elapsed time.Duration,
) {
	totalRows := 0
	for _, name := range order {
		totalRows += counts[name]
	}
	fmt.Fprintf(w, "subset %s: %s, %s in %s\n",
		cmd, pluralize(len(order), "table"), pluralize(totalRows, "row"),
		elapsed.Round(time.Millisecond))

	paths, extra := fkPaths(schema, counts, seed, forward)

	tableWidth, countWidth := 0, 0
	for _, name := range order {
		tableWidth = max(tableWidth, len(name))
		countWidth = max(countWidth, len(pluralize(counts[name], "row")))
	}
	for _, name := range order {
		line := fmt.Sprintf("  %-*s  %-*s", tableWidth, name, countWidth, pluralize(counts[name], "row"))
		switch {
		case name == seed:
			line += "  seed"
		case len(paths[name]) > 0:
			line += "  via " + strings.Join(paths[name], " → ")
			if extra[name] > 0 {
				line += fmt.Sprintf(" (+%s)", pluralize(extra[name], "more path"))
			}
		}
		fmt.Fprintln(w, line)
	}
}

// SummaryComment returns the leading SQL comment that prefixes normal (non-plan)
// output, e.g., "-- subset clone: 3 rows from 3 tables, parents-first".
func SummaryComment(cmd string, rows, tables int, forward bool) string {
	ordering := "children-first"
	if forward {
		ordering = "parents-first"
	}

	return fmt.Sprintf("-- subset %s: %s from %s, %s",
		cmd, pluralize(rows, "row"), pluralize(tables, "table"), ordering,
	)
}

// fkPaths computes, for every collected table, the shortest FK path from the
// seed (restricted to collected tables) and the number of extra direct
// references beyond the one on that path. Self-referencing FKs are ignored.
func fkPaths(
	schema introspect.Schema,
	counts map[string]int,
	seed string,
	forward bool,
) (paths map[string][]string, extra map[string]int) {
	adj := closureAdj(schema, counts, forward)

	// In-degree within the closure feeds the "+N more path" hint: how many
	// collected tables directly lead to a given table in the traversal.
	indeg := make(map[string]int)
	for _, neighbors := range adj {
		for _, nb := range neighbors {
			indeg[nb]++
		}
	}

	// BFS from the seed. Neighbors are sorted, so ties resolve deterministically.
	paths = map[string][]string{seed: {seed}}
	queue := []string{seed}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range adj[cur] {
			if _, ok := paths[nb]; ok {
				continue
			}
			next := append(slices.Clone(paths[cur]), nb)
			paths[nb] = next
			queue = append(queue, nb)
		}
	}

	extra = make(map[string]int)
	for table, d := range indeg {
		if d > 1 {
			extra[table] = d - 1
		}
	}

	return paths, extra
}

// closureAdj builds the traversal adjacency over collected tables only. For
// forward (clone) an edge points from a referencing table to the table it
// references; for backward (delete) the edge is reversed.
func closureAdj(schema introspect.Schema, counts map[string]int, forward bool) map[string][]string {
	inClosure := func(name string) bool {
		_, ok := counts[name]

		return ok
	}
	adj := make(map[string][]string)
	addEdge := func(from, to string) {
		if !slices.Contains(adj[from], to) {
			adj[from] = append(adj[from], to)
		}
	}
	for _, t := range schema.Tables {
		if !inClosure(t.Name) {
			continue
		}
		for _, fk := range t.ForeignKeys {
			if fk.ReferencedTable == t.Name || !inClosure(fk.ReferencedTable) {
				continue
			}
			if forward {
				addEdge(t.Name, fk.ReferencedTable)
			} else {
				addEdge(fk.ReferencedTable, t.Name)
			}
		}
	}
	for k := range adj {
		slices.Sort(adj[k])
	}

	return adj
}

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}

	return fmt.Sprintf("%d %ss", n, word)
}

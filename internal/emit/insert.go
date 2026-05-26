package emit

import (
	"fmt"
	"strings"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/introspect"
)

// BuildInsert renders a single INSERT statement for the given row. Columns
// marked IsGenerated are skipped because the database rejects writes that
// target them. Column order follows table.Columns to keep output stable.
// Row keys must match Column.Name; missing keys are treated as NULL.
func BuildInsert(d dialect.Dialect, table introspect.Table, row map[string]any) string {
	cols := make([]string, 0, len(table.Columns))
	vals := make([]string, 0, len(table.Columns))
	for _, c := range table.Columns {
		if c.IsGenerated {
			continue
		}
		cols = append(cols, d.QuoteIdent(c.Name))
		vals = append(vals, d.QuoteLiteral(row[c.Name]))
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		d.QuoteIdent(table.Name),
		strings.Join(cols, ", "),
		strings.Join(vals, ", "))
}

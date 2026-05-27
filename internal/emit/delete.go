package emit

import (
	"fmt"
	"strings"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/introspect"
)

// BuildDelete renders a single DELETE that targets one row by its primary
// key. Primary-key column order follows table.PrimaryKey, and composite keys
// are matched with an AND conjunction. Callers must ensure the table has a
// primary key (extract.Walk rejects tables without one). Row keys must match
// Column.Name.
func BuildDelete(d dialect.Dialect, table introspect.Table, row map[string]any) string {
	conds := make([]string, len(table.PrimaryKey))
	for i, col := range table.PrimaryKey {
		conds[i] = fmt.Sprintf("%s = %s", d.QuoteIdent(col), d.QuoteLiteral(row[col]))
	}

	return fmt.Sprintf("DELETE FROM %s WHERE %s;",
		d.QuoteIdent(table.Name),
		strings.Join(conds, " AND "),
	)
}

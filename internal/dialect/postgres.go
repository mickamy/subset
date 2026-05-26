package dialect

import (
	"fmt"
	"strings"
)

type Postgres struct{}

func (Postgres) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (Postgres) QuoteLiteral(_ any) string {
	panic("dialect.Postgres.QuoteLiteral: not yet implemented")
}

func (Postgres) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

package dialect

import "strings"

type MySQL struct{}

func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (MySQL) QuoteLiteral(_ any) string {
	panic("dialect.MySQL.QuoteLiteral: not yet implemented")
}

func (MySQL) Placeholder(_ int) string {
	return "?"
}

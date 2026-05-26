package dsn

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrMissingScheme = errors.New("missing scheme in DSN: expected mysql://, postgres://, or postgresql://")

func Scheme(dsn string) string {
	scheme, _, ok := strings.Cut(dsn, "://")
	if !ok {
		return ""
	}

	return scheme
}

// ToMySQLDSN converts a `mysql://user:pass@host:port/db?params` URI into
// the `user:pass@tcp(host:port)/db?params` format expected by
// go-sql-driver/mysql.
func ToMySQLDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse mysql DSN: %w", err)
	}
	if u.Scheme != "mysql" {
		return "", fmt.Errorf("expected mysql:// scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("mysql DSN missing host")
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" {
		return "", errors.New("mysql DSN missing database name")
	}

	var b strings.Builder
	if u.User != nil {
		// Username()/Password() return decoded values; String() preserves
		// URL escaping, which the driver would treat as literal characters.
		// go-sql-driver/mysql splits userinfo from host on the LAST `@`, so
		// a decoded password containing `@` round-trips correctly without
		// extra escaping.
		b.WriteString(u.User.Username())
		if pass, ok := u.User.Password(); ok {
			b.WriteByte(':')
			b.WriteString(pass)
		}
		b.WriteByte('@')
	}
	b.WriteString("tcp(")
	b.WriteString(u.Host)
	b.WriteString(")/")
	b.WriteString(db)
	if u.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(u.RawQuery)
	}

	return b.String(), nil
}

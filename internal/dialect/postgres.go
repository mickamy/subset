package dialect

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Postgres struct{}

func (Postgres) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// QuoteLiteral renders a Go value as a Postgres SQL literal. Strings are
// single-quoted with `'` escaped by doubling (standard_conforming_strings
// is assumed on; backslashes are literal). Bytea uses the hex format
// (`'\x..'`). Times are RFC3339Nano with single quotes; Postgres parses
// that for DATE / TIMESTAMP / TIMESTAMPTZ alike. Unknown types fall back
// to fmt.Sprint and are quoted as strings.
func (p Postgres) QuoteLiteral(v any) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case bool:
		if x {
			return "TRUE"
		}

		return "FALSE"
	case string:
		return p.quoteString(x)
	case []byte:
		return `'\x` + hex.EncodeToString(x) + "'"
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case time.Time:
		return p.quoteString(x.Format(time.RFC3339Nano))
	default:
		return p.quoteString(fmt.Sprint(x))
	}
}

func (Postgres) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

func (Postgres) quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

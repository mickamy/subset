package dialect

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type MySQL struct{}

func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteLiteral renders a Go value as a MySQL SQL literal. Strings escape
// `\` and `'` (MySQL interprets backslash sequences by default, unlike
// Postgres). Bytes use the hex literal form `X'..'`. Times are formatted
// as `'YYYY-MM-DD HH:MM:SS[.ffffff]'` in UTC; MySQL DATETIME / TIMESTAMP
// accept this for any precision up to microseconds. Unknown types fall
// back to fmt.Sprint and are quoted as strings.
func (m MySQL) QuoteLiteral(v any) string {
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
		return m.quoteString(x)
	case []byte:
		return "X'" + hex.EncodeToString(x) + "'"
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
		return m.quoteString(x.UTC().Format("2006-01-02 15:04:05.999999"))
	default:
		return m.quoteString(fmt.Sprint(x))
	}
}

func (MySQL) Placeholder(_ int) string {
	return "?"
}

func (MySQL) quoteString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)

	return "'" + s + "'"
}

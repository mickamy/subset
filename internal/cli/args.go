package cli

import (
	"flag"
	"strings"
)

// splitArgs separates flag tokens from positional tokens, supporting both
// `--flag=value` and `--flag value` forms. This lets callers place flags
// after positional arguments; the standard `flag` package stops at the
// first non-flag arg. `isBool` reports whether a flag name takes no value.
func splitArgs(args []string, isBool func(name string) bool) (flagArgs, posArgs []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			posArgs = append(posArgs, args[i+1:]...)

			return
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			flagArgs = append(flagArgs, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				i++

				continue
			}
			if !isBool(name) && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			i++
		default:
			posArgs = append(posArgs, arg)
			i++
		}
	}

	return
}

func isBoolFlag(fs *flag.FlagSet) func(name string) bool {
	return func(name string) bool {
		f := fs.Lookup(name)
		if f == nil {
			return false
		}
		bf, ok := f.Value.(interface{ IsBoolFlag() bool })

		return ok && bf.IsBoolFlag()
	}
}

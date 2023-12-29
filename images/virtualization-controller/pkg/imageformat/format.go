package imageformat

import "strings"

const FormatISO = "iso"

func IsISO(format string) bool {
	return strings.ToLower(format) == FormatISO
}

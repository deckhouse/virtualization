package humanize

import (
	"fmt"
	"math"
	"strconv"
)

const (
	BIBase = 1024
	SIBase = 1000
)

// Note: stop on zetta/zebi suffix as int64 max value is 2^63 -1 or 9,22Z.
var (
	BISuffixes = []string{"", "Ki", "Mi", "Gi", "Pi", "Ei", "Zi"}
	SISuffixes = []string{"", "K", "M", "G", "P", "E", "Z"}
)

// humanizeQuantity4 return 3 or 4 chars for num plus scale suffix.
func humanizeQuantity4(num int64, base int) string {
	var suffixes []string
	switch base {
	case BIBase:
		suffixes = BISuffixes
	case SIBase:
		suffixes = SISuffixes
	default:
		return strconv.FormatInt(num, 10)
	}

	sign := num < 0
	signStr := ""
	if sign {
		signStr = "-"
		if num == math.MinInt64 {
			num++
		}
		num = -num
	}

	// No suffix required.
	if num < int64(base) {
		return strconv.Itoa(int(num))
	}

	scale := 1
	var intPart int64
	var expPart int64
	for {
		expPart = num % int64(base)
		intPart = num / int64(base)

		if intPart < int64(base) || scale+1 >= len(suffixes) {
			break
		}
		// Discard exponent, work with integer part for next iteration.
		scale++
		num = intPart
	}

	expStr := fmt.Sprintf("%03d", expPart)

	// Now intPart is less than the base, scale is an index of the suffix.
	// Just print intPart and some first digits from the exponent part, so
	// overall string length become 3 or 4 chars.
	if intPart < 10 {
		return fmt.Sprintf("%s%d.%c%c%s", signStr, intPart, expStr[0], expStr[1], suffixes[scale])
	}
	if intPart < 100 {
		return fmt.Sprintf("%s%d.%c%s", signStr, intPart, expStr[0], suffixes[scale])
	}
	return fmt.Sprintf("%s%d%s", signStr, intPart, suffixes[scale])
}

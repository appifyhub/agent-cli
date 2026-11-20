package render

import (
	"math"
	"strconv"
	"strings"
)

// Credits truncates a credit amount to two decimal places.
// Decimal text avoids binary multiplication rounding the amount down incorrectly.
func Credits(balance float64) string {
	whole, fraction, _ := strings.Cut(strconv.FormatFloat(math.Abs(balance), 'f', -1, 64), ".")
	fraction += "00"
	credits := whole + "." + fraction[:2]
	if math.Signbit(balance) {
		credits = "-" + credits
	}
	return credits
}

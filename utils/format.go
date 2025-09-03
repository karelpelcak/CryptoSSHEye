package utils

import (
	"fmt"
	"strings"
)

func FormatFloatWithSpaces(f float64) string {
	s := fmt.Sprintf("%.2f", f)

	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := parts[1]

	var result []string
	for i, c := range reverse(intPart) {
		if i != 0 && i%3 == 0 {
			result = append(result, " ")
		}
		result = append(result, string(c))
	}

	intWithSpaces := reverse(strings.Join(result, ""))

	return intWithSpaces + "." + decPart
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

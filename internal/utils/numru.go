// utils/numru.go (или рядом)
package utils

import (
	"regexp"
	"strconv"
	"strings"
)

var rxKeepNums = regexp.MustCompile(`[^\d\.\-]`)

// ParseFloatRU парсит "1 234,50", "197 ,00", "2 345,6" (NBSP/NNBSP) и т.п.
func ParseFloatRU(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// убрать неразрывные/узкие пробелы и обычные пробелы
	repl := strings.NewReplacer("\u00A0", "", "\u202F", "", " ", "", "\t", "", ",", ".")
	s = repl.Replace(s)
	// оставить только цифры, точку и минус (на случай мусора)
	s = rxKeepNums.ReplaceAllString(s, "")
	if s == "" || s == "-" || s == "." {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

package sampling

import (
	"fmt"
	"strconv"
	"strings"
)

var sizeUnits = []struct {
	name string
	n    uint64
}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}}

// ParseSize reads a byte count like "1GB", "500 MB" or "1.5GB" (units of
// 1024, as Windows shows disk sizes); "0" is zero.
func ParseSize(s string) (uint64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	for _, u := range sizeUnits {
		if num, ok := strings.CutSuffix(t, u.name); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil || f < 0 {
				break
			}
			return uint64(f * float64(u.n)), nil
		}
	}
	if n, err := strconv.ParseUint(t, 10, 64); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("size %q: want a number with KB, MB, GB or TB, like \"1GB\"", s)
}

// FormatSize writes a byte count the way ParseSize reads it.
func FormatSize(n uint64) string {
	for _, u := range sizeUnits[:4] {
		if n >= u.n {
			v := float64(n) / float64(u.n)
			if v == float64(uint64(v)) {
				return fmt.Sprintf("%d%s", uint64(v), u.name)
			}
			return strconv.FormatFloat(v, 'f', 1, 64) + u.name
		}
	}
	return strconv.FormatUint(n, 10)
}

func formatSize(n uint64) string { return FormatSize(n) }

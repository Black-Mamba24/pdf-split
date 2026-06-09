package output

import (
	"fmt"
	"path/filepath"
)

func Names(input string, count int) []string {
	if count < 1 {
		return nil
	}

	base := filepath.Base(input)
	ext := filepath.Ext(base)
	stem := base[:len(base)-len(ext)]
	width := 3
	for n := count; n >= 1000; n /= 10 {
		width++
	}

	names := make([]string, count)
	for i := 1; i <= count; i++ {
		names[i-1] = fmt.Sprintf("%s-%0*d%s", stem, width, i, ext)
	}
	return names
}

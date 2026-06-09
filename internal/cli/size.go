package cli

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
)

var sizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)[[:space:]]*(KB|MB|GB)$`)

// ParseSize parses a positive decimal size using binary KB, MB, or GB units.
func ParseSize(input string) (int64, error) {
	matches := sizePattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(input)))
	if matches == nil {
		return 0, fmt.Errorf("invalid size %q: use a positive number followed by KB, MB, or GB", input)
	}

	value, ok := new(big.Rat).SetString(matches[1])
	if !ok || value.Sign() <= 0 {
		return 0, fmt.Errorf("invalid size %q", input)
	}

	multiplier := map[string]int64{
		"KB": 1 << 10,
		"MB": 1 << 20,
		"GB": 1 << 30,
	}[matches[2]]
	bytes := new(big.Rat).Mul(value, new(big.Rat).SetInt64(multiplier))
	if bytes.Cmp(new(big.Rat).SetInt64(math.MaxInt64)) > 0 {
		return 0, fmt.Errorf("size %q is too large", input)
	}

	quotient := new(big.Int).Quo(bytes.Num(), bytes.Denom())
	if quotient.Sign() <= 0 {
		return 0, fmt.Errorf("invalid size %q: value is less than one byte", input)
	}

	return quotient.Int64(), nil
}

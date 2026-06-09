package cli

import (
	"math"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := map[string]int64{
		"1KB":    1 << 10,
		"1kb":    1 << 10,
		"1.5MB":  1572864,
		"0.5GB":  536870912,
		" 2 Mb ": 2 << 20,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := ParseSize(input)
			if err != nil || got != want {
				t.Fatalf("ParseSize(%q) = %d, %v; want %d, nil", input, got, err, want)
			}
		})
	}
}

func TestParseSizeRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{
		"",
		"0MB",
		"0.0001KB",
		"-1MB",
		"1",
		"MB",
		"1TB",
		"NaNMB",
		"InfGB",
		"1.2.3MB",
		"9223372036854775807GB",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseSize(input); err == nil {
				t.Fatalf("ParseSize(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestParseSizeRejectsRoundedOverflow(t *testing.T) {
	for _, input := range []string{
		"8589934592GB",
		"8589934591.99999999930150806903839111328125GB", // MaxInt64 + 0.25 bytes.
	} {
		if _, err := ParseSize(input); err == nil {
			t.Fatalf("ParseSize(%q) unexpectedly succeeded; max int64 is %d", input, int64(math.MaxInt64))
		}
	}
}

func FuzzParseSizeNeverPanics(f *testing.F) {
	for _, seed := range []string{"1KB", "1.5MB", "", "1TB", "NaNMB", "8589934592GB"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseSize(input)
	})
}

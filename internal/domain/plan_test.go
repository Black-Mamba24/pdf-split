package domain

import (
	"encoding/binary"
	"testing"
)

func TestPageRangePages(t *testing.T) {
	if got := (PageRange{Start: 3, End: 7}).Pages(); got != 5 {
		t.Fatalf("Pages() = %d, want 5", got)
	}
}

func TestSplitPlanValidateAcceptsCompleteContiguousPlan(t *testing.T) {
	plan := SplitPlan{Ranges: []PageRange{
		{Start: 1, End: 2},
		{Start: 3, End: 5},
	}}

	if err := plan.Validate(5); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestSplitPlanValidateRejectsInvalidPlans(t *testing.T) {
	tests := []struct {
		name       string
		totalPages int
		plan       SplitPlan
	}{
		{name: "empty PDF", totalPages: 0, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 1}}}},
		{name: "negative total pages", totalPages: -1, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 1}}}},
		{name: "empty plan", totalPages: 5, plan: SplitPlan{}},
		{name: "first range starts after page one", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 2, End: 5}}}},
		{name: "range is empty", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 2}, {Start: 3, End: 2}, {Start: 3, End: 5}}}},
		{name: "ranges overlap", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 2}, {Start: 2, End: 5}}}},
		{name: "ranges have gap", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 2}, {Start: 4, End: 5}}}},
		{name: "contiguity check cannot overflow", totalPages: int(^uint(0) >> 1), plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: int(^uint(0) >> 1)}, {Start: -int(^uint(0)>>1) - 1, End: int(^uint(0) >> 1)}}}},
		{name: "range starts below bounds", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 0, End: 5}}}},
		{name: "range ends above bounds", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 6}}}},
		{name: "plan does not reach final page", totalPages: 5, plan: SplitPlan{Ranges: []PageRange{{Start: 1, End: 4}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.plan.Validate(tt.totalPages); err == nil {
				t.Fatalf("Validate(%d) accepted invalid plan: %#v", tt.totalPages, tt.plan)
			}
		})
	}
}

func FuzzSplitPlanValidateNeverPanics(f *testing.F) {
	f.Add(5, encodeRanges([]PageRange{{Start: 1, End: 2}, {Start: 3, End: 5}}))
	f.Add(0, []byte{})
	f.Add(-1, encodeRanges([]PageRange{{Start: -1, End: 1}}))

	f.Fuzz(func(t *testing.T, totalPages int, data []byte) {
		const maxRanges = 10_000
		if len(data) > maxRanges*16 {
			data = data[:maxRanges*16]
		}
		ranges := make([]PageRange, 0, len(data)/16)
		for len(data) >= 16 {
			ranges = append(ranges, PageRange{
				Start: int(int64(binary.LittleEndian.Uint64(data[:8]))),
				End:   int(int64(binary.LittleEndian.Uint64(data[8:16]))),
			})
			data = data[16:]
		}

		_ = (SplitPlan{Ranges: ranges}).Validate(totalPages)
	})
}

func encodeRanges(ranges []PageRange) []byte {
	data := make([]byte, len(ranges)*16)
	for i, pageRange := range ranges {
		binary.LittleEndian.PutUint64(data[i*16:i*16+8], uint64(int64(pageRange.Start)))
		binary.LittleEndian.PutUint64(data[i*16+8:i*16+16], uint64(int64(pageRange.End)))
	}
	return data
}

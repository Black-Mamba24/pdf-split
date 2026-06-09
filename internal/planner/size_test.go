package planner

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestByMaxSizeFitsAllPagesInOneOutput(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 20,
		{Start: 2, End: 2}: 20,
		{Start: 3, End: 3}: 20,
		{Start: 4, End: 4}: 20,
		{Start: 1, End: 4}: 90,
	}}

	result, err := ByMaxSize(context.Background(), 4, measurer, SizeOptions{MaxBytes: 100, LinearScan: 4})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, []domain.PageRange{{Start: 1, End: 4}})
	assertSizes(t, result.Sizes, []int64{90})
	if len(result.OversizedSingles) != 0 {
		t.Fatalf("OversizedSingles = %#v, want none", result.OversizedSingles)
	}
}

func TestByMaxSizeGreedyBoundarySearchProducesContinuousRanges(t *testing.T) {
	measurer := pageCountMeasurer{bytesPerPage: 10}

	result, err := ByMaxSize(context.Background(), 10, measurer, SizeOptions{MaxBytes: 35, LinearScan: 2})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	assertPlan(t, result.Plan, 10, []domain.PageRange{
		{Start: 1, End: 3},
		{Start: 4, End: 6},
		{Start: 7, End: 9},
		{Start: 10, End: 10},
	})
	assertSizes(t, result.Sizes, []int64{30, 30, 30, 10})
}

func TestByMaxSizeUsesSinglePageSizesToChooseBoundary(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 40,
		{Start: 2, End: 2}: 40,
		{Start: 3, End: 3}: 20,
		{Start: 4, End: 4}: 80,
		{Start: 1, End: 2}: 80,
		{Start: 1, End: 3}: 100,
		{Start: 1, End: 4}: 180,
	}}

	result, err := ByMaxSize(context.Background(), 4, measurer, SizeOptions{MaxBytes: 100, LinearScan: 1})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, []domain.PageRange{
		{Start: 1, End: 3},
		{Start: 4, End: 4},
	})
}

func TestByMaxSizeRecordsOversizedSinglePage(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 60,
		{Start: 2, End: 2}: 20,
		{Start: 2, End: 3}: 40,
	}}

	result, err := ByMaxSize(context.Background(), 3, measurer, SizeOptions{MaxBytes: 50, LinearScan: 3})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	assertPlan(t, result.Plan, 3, []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 3},
	})
	assertSizes(t, result.Sizes, []int64{60, 40})
	assertRanges(t, result.OversizedSingles, []domain.PageRange{{Start: 1, End: 1}})
}

func TestByMaxSizeMinimumPartsForcesMoreRanges(t *testing.T) {
	measurer := pageCountMeasurer{bytesPerPage: 10}

	result, err := ByMaxSize(context.Background(), 6, measurer, SizeOptions{MaxBytes: 100, MinimumParts: 3, LinearScan: 6})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	if len(result.Plan.Ranges) != 3 {
		t.Fatalf("ranges = %#v, want 3 ranges", result.Plan.Ranges)
	}
	if err := result.Plan.Validate(6); err != nil {
		t.Fatalf("plan invalid: %v", err)
	}
	for i, pageRange := range result.Plan.Ranges {
		if pageRange.Pages() < 1 {
			t.Fatalf("range %d is empty: %#v", i, pageRange)
		}
		if result.Sizes[i] > 100 && pageRange.Pages() > 1 {
			t.Fatalf("range %d size = %d, want <= 100", i, result.Sizes[i])
		}
	}
}

func TestByMaxSizeMeasurementBudgetExhaustionReturnsTypedError(t *testing.T) {
	_, err := ByMaxSize(context.Background(), 10, pageCountMeasurer{bytesPerPage: 10}, SizeOptions{
		MaxBytes:        30,
		LinearScan:      2,
		MaxMeasurements: 1,
	})
	if !errors.Is(err, ErrMeasurementBudget) {
		t.Fatalf("ByMaxSize() error = %v, want ErrMeasurementBudget", err)
	}
}

func TestByMaxSizeBudgetExhaustionAfterValidPlanReturnsBestPlan(t *testing.T) {
	result, err := ByMaxSize(context.Background(), 4, pageCountMeasurer{bytesPerPage: 10}, SizeOptions{
		MaxBytes:        100,
		MinimumParts:    3,
		LinearScan:      4,
		MaxMeasurements: 11,
	})
	if !errors.Is(err, ErrMeasurementBudget) {
		t.Fatalf("ByMaxSize() error = %v, want ErrMeasurementBudget", err)
	}
	if err := result.Plan.Validate(4); err != nil {
		t.Fatalf("best plan invalid: %v", err)
	}
}

func TestByMaxSizeUsesBoundedLinearScanForNonMonotonicMeasurements(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 10,
		{Start: 1, End: 2}: 90,
		{Start: 1, End: 3}: 25,
		{Start: 1, End: 4}: 120,
	}}

	result, err := ByMaxSize(context.Background(), 4, measurer, SizeOptions{MaxBytes: 50, LinearScan: 2})
	if err != nil {
		t.Fatalf("ByMaxSize() error = %v", err)
	}
	if got := result.Plan.Ranges[0]; got != (domain.PageRange{Start: 1, End: 3}) {
		t.Fatalf("first range = %#v, want 1-3 from linear scan", got)
	}
}

func TestByMaxSizeRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name       string
		totalPages int
		opts       SizeOptions
	}{
		{name: "non-positive pages", totalPages: 0, opts: SizeOptions{MaxBytes: 1}},
		{name: "non-positive max bytes", totalPages: 1, opts: SizeOptions{MaxBytes: 0}},
		{name: "negative minimum parts", totalPages: 1, opts: SizeOptions{MaxBytes: 1, MinimumParts: -1}},
		{name: "too many minimum parts", totalPages: 1, opts: SizeOptions{MaxBytes: 1, MinimumParts: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ByMaxSize(context.Background(), tt.totalPages, pageCountMeasurer{bytesPerPage: 1}, tt.opts); err == nil {
				t.Fatal("ByMaxSize() error = nil, want error")
			}
		})
	}
}

func TestByMaxSizeRandomizedInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 100; trial++ {
		totalPages := 1 + rng.Intn(25)
		maxBytes := int64(25 + rng.Intn(80))
		measurer := deterministicRangeMeasurer{seed: int64(trial)}

		result, err := ByMaxSize(context.Background(), totalPages, measurer, SizeOptions{MaxBytes: maxBytes, LinearScan: 4})
		if err != nil {
			t.Fatalf("trial %d: ByMaxSize() error = %v", trial, err)
		}
		if err := result.Plan.Validate(totalPages); err != nil {
			t.Fatalf("trial %d: plan invalid: %v", trial, err)
		}
		if len(result.Sizes) != len(result.Plan.Ranges) {
			t.Fatalf("trial %d: %d sizes for %d ranges", trial, len(result.Sizes), len(result.Plan.Ranges))
		}
		for i, pageRange := range result.Plan.Ranges {
			if pageRange.Pages() > 1 && result.Sizes[i] > maxBytes {
				t.Fatalf("trial %d: non-single range %#v size = %d, want <= %d", trial, pageRange, result.Sizes[i], maxBytes)
			}
		}
	}
}

type fakeRangeMeasurer struct {
	sizes map[domain.PageRange]int64
}

func (m fakeRangeMeasurer) Measure(_ context.Context, pageRange domain.PageRange) (int64, error) {
	if size, ok := m.sizes[pageRange]; ok {
		return size, nil
	}
	return int64(pageRange.Pages() * 100), nil
}

type pageCountMeasurer struct {
	bytesPerPage int64
}

func (m pageCountMeasurer) Measure(_ context.Context, pageRange domain.PageRange) (int64, error) {
	return int64(pageRange.Pages()) * m.bytesPerPage, nil
}

type deterministicRangeMeasurer struct {
	seed int64
}

func (m deterministicRangeMeasurer) Measure(_ context.Context, pageRange domain.PageRange) (int64, error) {
	pages := int64(pageRange.Pages())
	base := pages * (8 + (m.seed+int64(pageRange.Start))%9)
	jitter := int64((pageRange.Start*17 + pageRange.End*31 + int(m.seed)) % 19)
	if (pageRange.Start+pageRange.End+int(m.seed))%11 == 0 {
		return max(int64(1), base-jitter-pages*3), nil
	}
	return base + jitter, nil
}

func assertPlan(t *testing.T, plan domain.SplitPlan, totalPages int, want []domain.PageRange) {
	t.Helper()
	if err := plan.Validate(totalPages); err != nil {
		t.Fatalf("plan invalid: %v", err)
	}
	assertRanges(t, plan.Ranges, want)
}

func assertRanges(t *testing.T, got, want []domain.PageRange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ranges = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("range %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertSizes(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sizes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("size %d = %d, want %d", i, got[i], want[i])
		}
	}
}

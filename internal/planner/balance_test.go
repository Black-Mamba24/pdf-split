package planner

import (
	"context"
	"errors"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestBalanceMovesBoundaryToReduceLargestDeviation(t *testing.T) {
	measurer := pageCountMeasurer{bytesPerPage: 10}
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 5},
		{Start: 6, End: 6},
	}}

	result, err := Balance(context.Background(), plan, measurer, BalanceOptions{MaxBytes: 100, MaxIterations: 4})
	if err != nil {
		t.Fatalf("Balance() error = %v", err)
	}
	assertPlan(t, result.Plan, 6, []domain.PageRange{
		{Start: 1, End: 3},
		{Start: 4, End: 6},
	})
	assertSizes(t, result.Sizes, []int64{30, 30})
}

func TestBalanceNeverExceedsMaximum(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 20,
		{Start: 2, End: 4}: 45,
		{Start: 1, End: 2}: 60,
		{Start: 3, End: 4}: 30,
	}}
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
	}}

	result, err := Balance(context.Background(), plan, measurer, BalanceOptions{MaxBytes: 50, MaxIterations: 4})
	if err != nil {
		t.Fatalf("Balance() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, plan.Ranges)
	assertSizes(t, result.Sizes, []int64{20, 45})
}

func TestBalanceNeverMovesOversizedSinglePage(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 90,
		{Start: 2, End: 4}: 45,
		{Start: 1, End: 2}: 130,
		{Start: 3, End: 4}: 30,
	}}
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
	}}

	result, err := Balance(context.Background(), plan, measurer, BalanceOptions{MaxBytes: 50, MaxIterations: 4})
	if err != nil {
		t.Fatalf("Balance() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, plan.Ranges)
	assertSizes(t, result.Sizes, []int64{90, 45})
	assertRanges(t, result.OversizedSingles, []domain.PageRange{{Start: 1, End: 1}})
}

func TestBalanceNeverAbsorbsPageIntoOversizedSinglePage(t *testing.T) {
	measurer := fakeRangeMeasurer{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 200,
		{Start: 2, End: 4}: 20,
		{Start: 1, End: 2}: 40,
		{Start: 3, End: 4}: 30,
	}}
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
	}}

	result, err := Balance(context.Background(), plan, measurer, BalanceOptions{MaxBytes: 50, MaxIterations: 4})
	if err != nil {
		t.Fatalf("Balance() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, plan.Ranges)
	assertSizes(t, result.Sizes, []int64{200, 20})
	assertRanges(t, result.OversizedSingles, []domain.PageRange{{Start: 1, End: 1}})
}

func TestBalancePreservesRangeCountAndCoverage(t *testing.T) {
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 6},
		{Start: 7, End: 7},
		{Start: 8, End: 9},
	}}

	result, err := Balance(context.Background(), plan, pageCountMeasurer{bytesPerPage: 10}, BalanceOptions{MaxBytes: 100, MaxIterations: 10})
	if err != nil {
		t.Fatalf("Balance() error = %v", err)
	}
	if len(result.Plan.Ranges) != len(plan.Ranges) {
		t.Fatalf("range count = %d, want %d", len(result.Plan.Ranges), len(plan.Ranges))
	}
	if err := result.Plan.Validate(9); err != nil {
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

func TestBalanceStopsAtMeasurementBudget(t *testing.T) {
	plan := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 5},
		{Start: 6, End: 6},
	}}

	result, err := Balance(context.Background(), plan, pageCountMeasurer{bytesPerPage: 10}, BalanceOptions{
		MaxBytes:        100,
		MaxIterations:   4,
		MaxMeasurements: 2,
	})
	if !errors.Is(err, ErrMeasurementBudget) {
		t.Fatalf("Balance() error = %v, want ErrMeasurementBudget", err)
	}
	assertPlan(t, result.Plan, 6, plan.Ranges)
	assertSizes(t, result.Sizes, []int64{50, 10})
}

func TestBalanceRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		plan domain.SplitPlan
		opts BalanceOptions
	}{
		{name: "empty plan", opts: BalanceOptions{MaxBytes: 1}},
		{name: "non-positive max", plan: domain.SplitPlan{Ranges: []domain.PageRange{{Start: 1, End: 1}}}, opts: BalanceOptions{MaxBytes: 0}},
		{name: "invalid plan", plan: domain.SplitPlan{Ranges: []domain.PageRange{{Start: 2, End: 2}}}, opts: BalanceOptions{MaxBytes: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Balance(context.Background(), tt.plan, pageCountMeasurer{bytesPerPage: 1}, tt.opts); err == nil {
				t.Fatal("Balance() error = nil, want error")
			}
		})
	}
}

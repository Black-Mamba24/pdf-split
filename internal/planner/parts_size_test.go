package planner

import (
	"context"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestByBalancedPartsUsesMeasuredSizes(t *testing.T) {
	measurer := weightedPageMeasurer{weights: []int64{50, 50, 1, 1}}

	result, err := ByBalancedParts(context.Background(), 4, 2, measurer, 100)
	if err != nil {
		t.Fatalf("ByBalancedParts() error = %v", err)
	}
	assertPlan(t, result.Plan, 4, []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
	})
	assertSizes(t, result.Sizes, []int64{50, 52})
}

func TestByBalancedPartsKeepsExactCountAndCoverage(t *testing.T) {
	result, err := ByBalancedParts(context.Background(), 11, 4, pageCountMeasurer{bytesPerPage: 10}, 100)
	if err != nil {
		t.Fatalf("ByBalancedParts() error = %v", err)
	}
	if len(result.Plan.Ranges) != 4 {
		t.Fatalf("ranges = %d, want 4", len(result.Plan.Ranges))
	}
	if err := result.Plan.Validate(11); err != nil {
		t.Fatalf("plan invalid: %v", err)
	}
}

func TestByBalancedPartsHonorsMeasurementBudget(t *testing.T) {
	_, err := ByBalancedParts(context.Background(), 20, 3, pageCountMeasurer{bytesPerPage: 10}, 1)
	if err == nil {
		t.Fatal("ByBalancedParts() error = nil, want budget error")
	}
}

type weightedPageMeasurer struct {
	weights []int64
}

func (m weightedPageMeasurer) Measure(_ context.Context, pageRange domain.PageRange) (int64, error) {
	var size int64
	for page := pageRange.Start; page <= pageRange.End; page++ {
		size += m.weights[page-1]
	}
	return size, nil
}

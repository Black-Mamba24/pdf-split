package planner

import (
	"math"
	"reflect"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestByPartsDistributesRemainderToEarlierFiles(t *testing.T) {
	got, err := ByParts(10, 3)
	want := domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 4},
		{Start: 5, End: 7},
		{Start: 8, End: 10},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ByParts() = %#v, %v; want %#v", got, err, want)
	}
}

func TestByPartsRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name       string
		totalPages int
		parts      int
	}{
		{name: "non-positive total pages", totalPages: 0, parts: 1},
		{name: "negative total pages", totalPages: -1, parts: 1},
		{name: "non-positive parts", totalPages: 1, parts: 0},
		{name: "negative parts", totalPages: 1, parts: -1},
		{name: "more parts than pages", totalPages: 2, parts: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ByParts(tt.totalPages, tt.parts); err == nil {
				t.Fatal("ByParts() error = nil, want error")
			}
		})
	}
}

func TestByPartsHandlesMaximumPageCount(t *testing.T) {
	got, err := ByParts(math.MaxInt, 1)
	want := domain.SplitPlan{Ranges: []domain.PageRange{{Start: 1, End: math.MaxInt}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ByParts() = %#v, %v; want %#v", got, err, want)
	}
}

func TestByPartsExhaustiveDistribution(t *testing.T) {
	for totalPages := 1; totalPages <= 500; totalPages++ {
		for parts := 1; parts <= totalPages; parts++ {
			plan, err := ByParts(totalPages, parts)
			if err != nil {
				t.Fatalf("ByParts(%d, %d) error = %v", totalPages, parts, err)
			}
			if err := plan.Validate(totalPages); err != nil {
				t.Fatalf("ByParts(%d, %d) returned invalid plan: %v", totalPages, parts, err)
			}
			if len(plan.Ranges) != parts {
				t.Fatalf("ByParts(%d, %d) returned %d ranges, want %d", totalPages, parts, len(plan.Ranges), parts)
			}

			base := totalPages / parts
			remainder := totalPages % parts
			minPages, maxPages := totalPages, 0
			for i, pageRange := range plan.Ranges {
				gotPages := pageRange.Pages()
				wantPages := base
				if i < remainder {
					wantPages++
				}
				if gotPages != wantPages {
					t.Fatalf("ByParts(%d, %d) range %d has %d pages, want %d", totalPages, parts, i, gotPages, wantPages)
				}
				minPages = min(minPages, gotPages)
				maxPages = max(maxPages, gotPages)
			}
			if maxPages-minPages > 1 {
				t.Fatalf("ByParts(%d, %d) page-count difference = %d, want <= 1", totalPages, parts, maxPages-minPages)
			}
		}
	}
}

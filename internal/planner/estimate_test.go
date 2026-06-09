package planner

import "testing"

func TestByEstimatedMaxSizeUsesInputRatioAndMinimumParts(t *testing.T) {
	tests := []struct {
		name         string
		totalPages   int
		inputBytes   int64
		maxBytes     int64
		minimumParts int
		wantParts    int
	}{
		{name: "ratio", totalPages: 100, inputBytes: 269, maxBytes: 90, wantParts: 3},
		{name: "minimum parts", totalPages: 100, inputBytes: 100, maxBytes: 90, minimumParts: 4, wantParts: 4},
		{name: "never exceeds pages", totalPages: 2, inputBytes: 1000, maxBytes: 1, wantParts: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ByEstimatedMaxSize(tt.totalPages, tt.inputBytes, tt.maxBytes, tt.minimumParts)
			if err != nil {
				t.Fatalf("ByEstimatedMaxSize() error = %v", err)
			}
			if len(plan.Ranges) != tt.wantParts {
				t.Fatalf("parts = %d, want %d", len(plan.Ranges), tt.wantParts)
			}
			if err := plan.Validate(tt.totalPages); err != nil {
				t.Fatalf("plan invalid: %v", err)
			}
		})
	}
}

package planner

import (
	"fmt"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func ByEstimatedMaxSize(totalPages int, inputBytes, maxBytes int64, minimumParts int) (domain.SplitPlan, error) {
	if totalPages < 1 {
		return domain.SplitPlan{}, fmt.Errorf("total pages must be positive, got %d", totalPages)
	}
	if inputBytes < 1 {
		return domain.SplitPlan{}, fmt.Errorf("input bytes must be positive, got %d", inputBytes)
	}
	if maxBytes < 1 {
		return domain.SplitPlan{}, fmt.Errorf("max bytes must be positive, got %d", maxBytes)
	}
	if minimumParts < 0 || minimumParts > totalPages {
		return domain.SplitPlan{}, fmt.Errorf("minimum parts must be between 0 and %d, got %d", totalPages, minimumParts)
	}

	parts := int(inputBytes / maxBytes)
	if inputBytes%maxBytes != 0 {
		parts++
	}
	if parts < minimumParts {
		parts = minimumParts
	}
	if parts < 1 {
		parts = 1
	}
	if parts > totalPages {
		parts = totalPages
	}
	return ByParts(totalPages, parts)
}

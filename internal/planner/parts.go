package planner

import (
	"fmt"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func ByParts(totalPages, parts int) (domain.SplitPlan, error) {
	if totalPages < 1 {
		return domain.SplitPlan{}, fmt.Errorf("total pages must be positive, got %d", totalPages)
	}
	if parts < 1 || parts > totalPages {
		return domain.SplitPlan{}, fmt.Errorf("parts must be between 1 and %d, got %d", totalPages, parts)
	}

	base, remainder := totalPages/parts, totalPages%parts
	ranges := make([]domain.PageRange, 0, parts)
	nextPage := 1
	for i := 0; i < parts; i++ {
		pageCount := base
		if i < remainder {
			pageCount++
		}
		ranges = append(ranges, domain.PageRange{
			Start: nextPage,
			End:   nextPage + pageCount - 1,
		})
		if i+1 < parts {
			nextPage += pageCount
		}
	}

	plan := domain.SplitPlan{Ranges: ranges}
	return plan, plan.Validate(totalPages)
}

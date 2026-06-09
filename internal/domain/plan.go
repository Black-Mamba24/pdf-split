package domain

import "fmt"

type PageRange struct {
	Start int
	End   int
}

func (r PageRange) Pages() int {
	return r.End - r.Start + 1
}

type SplitPlan struct {
	Ranges []PageRange
}

func (p SplitPlan) Validate(totalPages int) error {
	if totalPages < 1 {
		return fmt.Errorf("total pages must be positive, got %d", totalPages)
	}
	if len(p.Ranges) == 0 {
		return fmt.Errorf("split plan must contain at least one range")
	}

	for i, pageRange := range p.Ranges {
		if pageRange.Start < 1 || pageRange.End > totalPages {
			return fmt.Errorf("range %d is outside page bounds 1-%d: %d-%d", i+1, totalPages, pageRange.Start, pageRange.End)
		}
		if pageRange.End < pageRange.Start {
			return fmt.Errorf("range %d is empty: %d-%d", i+1, pageRange.Start, pageRange.End)
		}
		if i == 0 {
			if pageRange.Start != 1 {
				return fmt.Errorf("first range starts at page %d, want 1", pageRange.Start)
			}
			continue
		}

		previous := p.Ranges[i-1]
		if pageRange.Start-1 != previous.End {
			return fmt.Errorf("range %d is not contiguous with range %d", i+1, i)
		}
	}

	lastPage := p.Ranges[len(p.Ranges)-1].End
	if lastPage != totalPages {
		return fmt.Errorf("split plan ends at page %d, want %d", lastPage, totalPages)
	}

	return nil
}

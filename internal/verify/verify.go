package verify

import (
	"fmt"
	"os"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

type Inspector interface {
	Inspect(path string) (pdf.Info, error)
}

type Request struct {
	TotalPages       int
	Plan             domain.SplitPlan
	Paths            []string
	MaxBytes         int64
	OversizedSingles map[domain.PageRange]struct{}
}

func Verify(inspector Inspector, req Request) error {
	if err := req.Plan.Validate(req.TotalPages); err != nil {
		return err
	}
	if len(req.Paths) != len(req.Plan.Ranges) {
		return fmt.Errorf("got %d output paths for %d planned ranges", len(req.Paths), len(req.Plan.Ranges))
	}

	summedPages := 0
	for i, path := range req.Paths {
		pageRange := req.Plan.Ranges[i]
		info, err := inspector.Inspect(path)
		if err != nil {
			return fmt.Errorf("inspect output %q for pages %d-%d: %w", path, pageRange.Start, pageRange.End, err)
		}
		wantPages := pageRange.Pages()
		if info.Pages != wantPages {
			return fmt.Errorf("output %q has %d pages, want %d for planned range %d-%d", path, info.Pages, wantPages, pageRange.Start, pageRange.End)
		}
		summedPages += info.Pages

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat output %q: %w", path, err)
		}
		if req.MaxBytes > 0 && stat.Size() > req.MaxBytes && !allowedOversizedSingle(pageRange, req.OversizedSingles) {
			return fmt.Errorf("output %q for pages %d-%d is %d bytes, exceeds limit %d", path, pageRange.Start, pageRange.End, stat.Size(), req.MaxBytes)
		}
	}

	if summedPages != req.TotalPages {
		return fmt.Errorf("outputs contain %d total pages, want %d", summedPages, req.TotalPages)
	}
	return nil
}

func allowedOversizedSingle(pageRange domain.PageRange, oversizedSingles map[domain.PageRange]struct{}) bool {
	if pageRange.Pages() != 1 {
		return false
	}
	_, ok := oversizedSingles[pageRange]
	return ok
}

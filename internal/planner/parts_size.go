package planner

import (
	"context"
	"fmt"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func ByBalancedParts(ctx context.Context, totalPages, parts int, measurer RangeMeasurer, maxMeasurements int) (SizeResult, error) {
	if totalPages < 1 {
		return SizeResult{}, fmt.Errorf("total pages must be positive, got %d", totalPages)
	}
	if parts < 1 || parts > totalPages {
		return SizeResult{}, fmt.Errorf("parts must be between 1 and %d, got %d", totalPages, parts)
	}

	p := balancedPartsPlanner{ctx: ctx, measurer: measurer, maxMeasurements: maxMeasurements}
	result := SizeResult{}
	start := 1
	for part := 0; part < parts-1; part++ {
		partsLeft := parts - part
		remaining := domain.PageRange{Start: start, End: totalPages}
		remainingSize, err := p.measure(remaining)
		if err != nil {
			return result, err
		}
		target := remainingSize / int64(partsLeft)
		maxEnd := totalPages - (partsLeft - 1)
		pageRange, size, err := p.closestRange(start, maxEnd, target)
		if err != nil {
			return result, err
		}
		result.Plan.Ranges = append(result.Plan.Ranges, pageRange)
		result.Sizes = append(result.Sizes, size)
		start = pageRange.End + 1
	}

	last := domain.PageRange{Start: start, End: totalPages}
	lastSize, err := p.measure(last)
	if err != nil {
		return result, err
	}
	result.Plan.Ranges = append(result.Plan.Ranges, last)
	result.Sizes = append(result.Sizes, lastSize)
	return result, result.Plan.Validate(totalPages)
}

type balancedPartsPlanner struct {
	ctx             context.Context
	measurer        RangeMeasurer
	maxMeasurements int
	measurements    int
}

func (p *balancedPartsPlanner) closestRange(start, maxEnd int, target int64) (domain.PageRange, int64, error) {
	low, high := start, maxEnd
	best := domain.PageRange{Start: start, End: start}
	bestSize := int64(0)
	bestDiff := int64(^uint64(0) >> 1)
	for low <= high {
		mid := low + (high-low)/2
		candidate := domain.PageRange{Start: start, End: mid}
		size, err := p.measure(candidate)
		if err != nil {
			return domain.PageRange{}, 0, err
		}
		diff := abs64(size - target)
		if diff < bestDiff || diff == bestDiff && candidate.End < best.End {
			best, bestSize, bestDiff = candidate, size, diff
		}
		if size < target {
			low = mid + 1
		} else if size > target {
			high = mid - 1
		} else {
			break
		}
	}
	return best, bestSize, nil
}

func (p *balancedPartsPlanner) measure(pageRange domain.PageRange) (int64, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	if p.maxMeasurements > 0 && p.measurements >= p.maxMeasurements {
		return 0, ErrMeasurementBudget
	}
	p.measurements++
	return p.measurer.Measure(p.ctx, pageRange)
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

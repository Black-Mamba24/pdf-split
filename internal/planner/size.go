package planner

import (
	"context"
	"errors"
	"fmt"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

var ErrMeasurementBudget = errors.New("measurement budget exhausted")

type RangeMeasurer interface {
	Measure(context.Context, domain.PageRange) (int64, error)
}

type SizeOptions struct {
	MaxBytes        int64
	MinimumParts    int
	LinearScan      int
	MaxMeasurements int
}

type SizeResult struct {
	Plan             domain.SplitPlan
	Sizes            []int64
	OversizedSingles []domain.PageRange
}

type sizePlanner struct {
	ctx             context.Context
	totalPages      int
	measurer        RangeMeasurer
	opts            SizeOptions
	measurements    int
	budgetExhausted bool
}

func ByMaxSize(ctx context.Context, totalPages int, measurer RangeMeasurer, opts SizeOptions) (SizeResult, error) {
	if totalPages < 1 {
		return SizeResult{}, fmt.Errorf("total pages must be positive, got %d", totalPages)
	}
	if opts.MaxBytes < 1 {
		return SizeResult{}, fmt.Errorf("max bytes must be positive, got %d", opts.MaxBytes)
	}
	if opts.MinimumParts < 0 || opts.MinimumParts > totalPages {
		return SizeResult{}, fmt.Errorf("minimum parts must be between 0 and %d, got %d", totalPages, opts.MinimumParts)
	}
	if opts.MinimumParts == 0 {
		opts.MinimumParts = 1
	}
	if opts.LinearScan < 1 {
		opts.LinearScan = 1
	}

	planner := sizePlanner{ctx: ctx, totalPages: totalPages, measurer: measurer, opts: opts}
	result, err := planner.greedyPlan()
	if err != nil {
		return result, err
	}

	result, err = planner.ensureMinimumParts(result)
	if err != nil {
		return result, err
	}
	if planner.budgetExhausted {
		return result, ErrMeasurementBudget
	}
	return result, nil
}

func (p *sizePlanner) greedyPlan() (SizeResult, error) {
	var result SizeResult
	for start := 1; start <= p.totalPages; {
		if err := p.ctx.Err(); err != nil {
			return result, err
		}

		pageRange, size, oversized, err := p.largestRangeFrom(start)
		if err != nil {
			return result, err
		}
		result.Plan.Ranges = append(result.Plan.Ranges, pageRange)
		result.Sizes = append(result.Sizes, size)
		if oversized {
			result.OversizedSingles = append(result.OversizedSingles, pageRange)
		}
		start = pageRange.End + 1
	}

	if err := result.Plan.Validate(p.totalPages); err != nil {
		return result, err
	}
	return result, nil
}

func (p *sizePlanner) largestRangeFrom(start int) (domain.PageRange, int64, bool, error) {
	single := domain.PageRange{Start: start, End: start}
	singleSize, err := p.measure(single)
	if err != nil {
		return domain.PageRange{}, 0, false, err
	}
	if start == p.totalPages || singleSize > p.opts.MaxBytes {
		return single, singleSize, singleSize > p.opts.MaxBytes, nil
	}

	low, lowSize := start, singleSize
	high := start
	step := 1
	for high < p.totalPages {
		next := high + step
		if next > p.totalPages {
			next = p.totalPages
		}
		size, err := p.measure(domain.PageRange{Start: start, End: next})
		if err != nil {
			return domain.PageRange{}, 0, false, err
		}
		if size <= p.opts.MaxBytes {
			low, lowSize = next, size
			if next == p.totalPages {
				return domain.PageRange{Start: start, End: low}, lowSize, false, nil
			}
			high = next
			step *= 2
			continue
		}
		high = next
		break
	}

	left, right := low+1, high-1
	for left <= right {
		mid := left + (right-left)/2
		size, err := p.measure(domain.PageRange{Start: start, End: mid})
		if err != nil {
			return domain.PageRange{}, 0, false, err
		}
		if size <= p.opts.MaxBytes {
			low, lowSize = mid, size
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	low, lowSize, err = p.linearScanBest(start, low, lowSize)
	if err != nil {
		return domain.PageRange{}, 0, false, err
	}
	return domain.PageRange{Start: start, End: low}, lowSize, false, nil
}

func (p *sizePlanner) linearScanBest(start, currentEnd int, currentSize int64) (int, int64, error) {
	bestEnd, bestSize := currentEnd, currentSize
	from := currentEnd - p.opts.LinearScan
	if from < start {
		from = start
	}
	to := currentEnd + p.opts.LinearScan
	if to > p.totalPages {
		to = p.totalPages
	}

	for end := from; end <= to; end++ {
		size, err := p.measure(domain.PageRange{Start: start, End: end})
		if err != nil {
			return 0, 0, err
		}
		if size <= p.opts.MaxBytes && (end > bestEnd || end == bestEnd && size < bestSize) {
			bestEnd, bestSize = end, size
		}
	}
	return bestEnd, bestSize, nil
}

func (p *sizePlanner) ensureMinimumParts(result SizeResult) (SizeResult, error) {
	for len(result.Plan.Ranges) < p.opts.MinimumParts {
		index := largestMultiPageRange(result.Plan.Ranges)
		if index < 0 {
			return result, fmt.Errorf("cannot split %d pages into %d parts", p.totalPages, p.opts.MinimumParts)
		}

		left, right := splitRange(result.Plan.Ranges[index])
		leftSize, err := p.measure(left)
		if err != nil {
			if errors.Is(err, ErrMeasurementBudget) {
				p.budgetExhausted = true
				return result, nil
			}
			return result, err
		}
		rightSize, err := p.measure(right)
		if err != nil {
			if errors.Is(err, ErrMeasurementBudget) {
				p.budgetExhausted = true
				return result, nil
			}
			return result, err
		}
		if !splitChildAllowed(left, leftSize, p.opts.MaxBytes) || !splitChildAllowed(right, rightSize, p.opts.MaxBytes) {
			return result, fmt.Errorf("cannot satisfy minimum parts without oversizing split %d-%d", result.Plan.Ranges[index].Start, result.Plan.Ranges[index].End)
		}

		result.Plan.Ranges = replaceRange(result.Plan.Ranges, index, left, right)
		result.Sizes = replaceSize(result.Sizes, index, leftSize, rightSize)
	}
	if err := result.Plan.Validate(p.totalPages); err != nil {
		return result, err
	}
	return result, nil
}

func (p *sizePlanner) measure(pageRange domain.PageRange) (int64, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	if p.opts.MaxMeasurements > 0 && p.measurements >= p.opts.MaxMeasurements {
		return 0, ErrMeasurementBudget
	}
	p.measurements++
	return p.measurer.Measure(p.ctx, pageRange)
}

func largestMultiPageRange(ranges []domain.PageRange) int {
	index := -1
	maxPages := 0
	for i, pageRange := range ranges {
		if pages := pageRange.Pages(); pages > maxPages && pages > 1 {
			index = i
			maxPages = pages
		}
	}
	return index
}

func splitRange(pageRange domain.PageRange) (domain.PageRange, domain.PageRange) {
	mid := pageRange.Start + (pageRange.Pages()/2 - 1)
	return domain.PageRange{Start: pageRange.Start, End: mid}, domain.PageRange{Start: mid + 1, End: pageRange.End}
}

func splitChildAllowed(pageRange domain.PageRange, size, maxBytes int64) bool {
	return pageRange.Pages() == 1 || size <= maxBytes
}

func replaceRange(ranges []domain.PageRange, index int, left, right domain.PageRange) []domain.PageRange {
	replaced := make([]domain.PageRange, 0, len(ranges)+1)
	replaced = append(replaced, ranges[:index]...)
	replaced = append(replaced, left, right)
	replaced = append(replaced, ranges[index+1:]...)
	return replaced
}

func replaceSize(sizes []int64, index int, left, right int64) []int64 {
	replaced := make([]int64, 0, len(sizes)+1)
	replaced = append(replaced, sizes[:index]...)
	replaced = append(replaced, left, right)
	replaced = append(replaced, sizes[index+1:]...)
	return replaced
}

package planner

import (
	"context"
	"fmt"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

type BalanceOptions struct {
	MaxBytes        int64
	MaxIterations   int
	MaxMeasurements int
}

type balancePlanner struct {
	ctx          context.Context
	measurer     RangeMeasurer
	opts         BalanceOptions
	measurements int
}

func Balance(ctx context.Context, plan domain.SplitPlan, measurer RangeMeasurer, opts BalanceOptions) (SizeResult, error) {
	if opts.MaxBytes < 1 {
		return SizeResult{}, fmt.Errorf("max bytes must be positive, got %d", opts.MaxBytes)
	}
	if len(plan.Ranges) == 0 {
		return SizeResult{}, fmt.Errorf("split plan must contain at least one range")
	}
	totalPages := plan.Ranges[len(plan.Ranges)-1].End
	if err := plan.Validate(totalPages); err != nil {
		return SizeResult{}, err
	}
	if opts.MaxIterations < 1 {
		opts.MaxIterations = 1
	}

	planner := balancePlanner{ctx: ctx, measurer: measurer, opts: opts}
	result := SizeResult{Plan: clonePlan(plan)}
	var err error
	result.Sizes, result.OversizedSingles, err = planner.measurePlan(result.Plan)
	if err != nil {
		return result, err
	}

	for iteration := 0; iteration < opts.MaxIterations; iteration++ {
		move, ok, err := planner.bestMove(result)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}

		result.Plan.Ranges[move.leftIndex] = move.left
		result.Plan.Ranges[move.rightIndex] = move.right
		result.Sizes[move.leftIndex] = move.leftSize
		result.Sizes[move.rightIndex] = move.rightSize
		result.OversizedSingles = oversizedSingles(result.Plan.Ranges, result.Sizes, opts.MaxBytes)
		if err := result.Plan.Validate(totalPages); err != nil {
			return result, err
		}
	}
	return result, nil
}

type boundaryMove struct {
	leftIndex  int
	rightIndex int
	left       domain.PageRange
	right      domain.PageRange
	leftSize   int64
	rightSize  int64
	quality    int64
}

func (p *balancePlanner) bestMove(result SizeResult) (boundaryMove, bool, error) {
	currentQuality := sizeDeviation(result.Sizes)
	var best boundaryMove
	found := false

	for i := 0; i+1 < len(result.Plan.Ranges); i++ {
		if isOversizedSingle(result.Plan.Ranges[i], result.Sizes[i], p.opts.MaxBytes) ||
			isOversizedSingle(result.Plan.Ranges[i+1], result.Sizes[i+1], p.opts.MaxBytes) {
			continue
		}
		candidates := candidateMoves(result.Plan.Ranges[i], result.Plan.Ranges[i+1])
		for _, candidate := range candidates {
			leftSize, err := p.measure(candidate.left)
			if err != nil {
				return boundaryMove{}, false, err
			}
			rightSize, err := p.measure(candidate.right)
			if err != nil {
				return boundaryMove{}, false, err
			}
			if !balancedRangeAllowed(candidate.left, leftSize, p.opts.MaxBytes) || !balancedRangeAllowed(candidate.right, rightSize, p.opts.MaxBytes) {
				continue
			}

			candidateSizes := append([]int64(nil), result.Sizes...)
			candidateSizes[i] = leftSize
			candidateSizes[i+1] = rightSize
			quality := sizeDeviation(candidateSizes)
			if quality >= currentQuality {
				continue
			}
			if !found || quality < best.quality {
				candidate.leftIndex = i
				candidate.rightIndex = i + 1
				candidate.leftSize = leftSize
				candidate.rightSize = rightSize
				candidate.quality = quality
				best = candidate
				found = true
			}
		}
	}
	return best, found, nil
}

func isOversizedSingle(pageRange domain.PageRange, size, maxBytes int64) bool {
	return pageRange.Pages() == 1 && size > maxBytes
}

func (p *balancePlanner) measurePlan(plan domain.SplitPlan) ([]int64, []domain.PageRange, error) {
	sizes := make([]int64, len(plan.Ranges))
	for i, pageRange := range plan.Ranges {
		size, err := p.measure(pageRange)
		if err != nil {
			return sizes, nil, err
		}
		sizes[i] = size
	}
	return sizes, oversizedSingles(plan.Ranges, sizes, p.opts.MaxBytes), nil
}

func (p *balancePlanner) measure(pageRange domain.PageRange) (int64, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	if p.opts.MaxMeasurements > 0 && p.measurements >= p.opts.MaxMeasurements {
		return 0, ErrMeasurementBudget
	}
	p.measurements++
	return p.measurer.Measure(p.ctx, pageRange)
}

func candidateMoves(left, right domain.PageRange) []boundaryMove {
	moves := make([]boundaryMove, 0, 2)
	if left.Pages() > 1 {
		moves = append(moves, boundaryMove{
			left:  domain.PageRange{Start: left.Start, End: left.End - 1},
			right: domain.PageRange{Start: left.End, End: right.End},
		})
	}
	if right.Pages() > 1 {
		moves = append(moves, boundaryMove{
			left:  domain.PageRange{Start: left.Start, End: right.Start},
			right: domain.PageRange{Start: right.Start + 1, End: right.End},
		})
	}
	return moves
}

func balancedRangeAllowed(pageRange domain.PageRange, size, maxBytes int64) bool {
	return pageRange.Pages() == 1 || size <= maxBytes
}

func oversizedSingles(ranges []domain.PageRange, sizes []int64, maxBytes int64) []domain.PageRange {
	var oversized []domain.PageRange
	for i, pageRange := range ranges {
		if pageRange.Pages() == 1 && sizes[i] > maxBytes {
			oversized = append(oversized, pageRange)
		}
	}
	return oversized
}

func sizeDeviation(sizes []int64) int64 {
	if len(sizes) == 0 {
		return 0
	}
	minSize, maxSize := sizes[0], sizes[0]
	for _, size := range sizes[1:] {
		if size < minSize {
			minSize = size
		}
		if size > maxSize {
			maxSize = size
		}
	}
	return maxSize - minSize
}

func clonePlan(plan domain.SplitPlan) domain.SplitPlan {
	return domain.SplitPlan{Ranges: append([]domain.PageRange(nil), plan.Ranges...)}
}

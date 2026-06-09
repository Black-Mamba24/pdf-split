package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/measure"
	"github.com/Black-Mamba24/pdf-split/internal/output"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
	"github.com/Black-Mamba24/pdf-split/internal/planner"
	"github.com/Black-Mamba24/pdf-split/internal/progress"
	"github.com/Black-Mamba24/pdf-split/internal/verify"
)

type Options struct {
	Input      string
	Parts      int
	MaxSize    int64
	OutputDir  string
	Overwrite  bool
	NoProgress bool
}

type Transaction interface {
	StagePath(index int) string
	Commit() error
	Abort() error
}

type Dependencies struct {
	Engine      pdf.Engine
	Begin       func(outputDir string, names []string, overwrite bool) (Transaction, error)
	Verify      func(verify.Inspector, verify.Request) error
	NewReporter func(noProgress bool) progress.Reporter
	Stdout      io.Writer
	Stderr      io.Writer
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 8
}

func Run(ctx context.Context, opts Options, deps Dependencies) error {
	deps = withDefaults(deps)
	if err := validateOptions(opts); err != nil {
		return exit(2, err)
	}
	if err := ctx.Err(); err != nil {
		return exit(8, err)
	}

	info, err := deps.Engine.Inspect(opts.Input)
	if err != nil {
		return classifyInputError(err)
	}
	if opts.Parts > info.Pages {
		return exit(2, fmt.Errorf("--parts must be between 1 and %d, got %d", info.Pages, opts.Parts))
	}

	reporter := deps.NewReporter(opts.NoProgress)
	defer reporter.Close()

	plan, sizes, oversized, err := createPlan(ctx, opts, info.Pages, deps.Engine, reporter)
	if err != nil {
		return classifyPlanningError(err)
	}
	for attempts := 0; attempts <= info.Pages; attempts++ {
		err = runAttempt(ctx, opts, deps, reporter, info.Pages, plan, sizes, oversized)
		if err == nil {
			for _, pageRange := range oversized {
				fmt.Fprintf(deps.Stderr, "warning: page %d exceeds maximum size and was emitted alone\n", pageRange.Start)
			}
			fmt.Fprintf(deps.Stdout, "split %d pages into %d files in %s (%d warnings)\n", info.Pages, len(plan.Ranges), opts.OutputDir, len(oversized))
			return nil
		}

		var sizeErr *verify.SizeLimitError
		if !errors.As(err, &sizeErr) || opts.MaxSize == 0 {
			return err
		}
		if sizeErr.Range.Pages() == 1 {
			oversized = appendUniqueRange(oversized, sizeErr.Range)
			continue
		}
		plan, err = splitPlanRange(plan, info.Pages, sizeErr.Range)
		if err != nil {
			return exit(5, err)
		}
		sizes = nil
	}
	return exit(5, errors.New("could not satisfy maximum size after replanning"))
}

func runAttempt(ctx context.Context, opts Options, deps Dependencies, reporter progress.Reporter, totalPages int, plan domain.SplitPlan, sizes []int64, oversized []domain.PageRange) error {
	names := output.Names(opts.Input, len(plan.Ranges))
	tx, err := deps.Begin(opts.OutputDir, names, opts.Overwrite)
	if err != nil {
		return exit(6, err)
	}
	defer tx.Abort()

	paths := make([]string, len(plan.Ranges))
	for i, pageRange := range plan.Ranges {
		if err := ctx.Err(); err != nil {
			return exit(8, err)
		}
		path := tx.StagePath(i)
		paths[i] = path
		expected := int64(0)
		if i < len(sizes) {
			expected = sizes[i]
		}
		reporter.StartFile(i+1, len(plan.Ranges), names[i], expected)
		done := make(chan struct{})
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			reporter.WatchFile(ctx, path, done)
		}()
		writeErr := deps.Engine.WriteRange(opts.Input, path, pageRange)
		close(done)
		<-watchDone
		if writeErr != nil {
			return classifyWriteError(writeErr)
		}
		stat, statErr := os.Stat(path)
		if statErr != nil {
			return exit(6, fmt.Errorf("stat generated output %q: %w", path, statErr))
		}
		reporter.Complete(stat.Size())
	}

	oversizedSet := make(map[domain.PageRange]struct{}, len(oversized))
	for _, pageRange := range oversized {
		oversizedSet[pageRange] = struct{}{}
	}
	if err := deps.Verify(deps.Engine, verify.Request{
		TotalPages:       totalPages,
		Plan:             plan,
		Paths:            paths,
		MaxBytes:         opts.MaxSize,
		OversizedSingles: oversizedSet,
	}); err != nil {
		var sizeErr *verify.SizeLimitError
		if errors.As(err, &sizeErr) {
			return sizeErr
		}
		return exit(7, err)
	}
	if err := tx.Commit(); err != nil {
		return exit(6, err)
	}
	return nil
}

func splitPlanRange(plan domain.SplitPlan, totalPages int, target domain.PageRange) (domain.SplitPlan, error) {
	if target.Pages() < 2 {
		return domain.SplitPlan{}, fmt.Errorf("cannot split single-page range %d-%d", target.Start, target.End)
	}
	for i, pageRange := range plan.Ranges {
		if pageRange != target {
			continue
		}
		mid := pageRange.Start + (pageRange.Pages()/2 - 1)
		ranges := make([]domain.PageRange, 0, len(plan.Ranges)+1)
		ranges = append(ranges, plan.Ranges[:i]...)
		ranges = append(ranges,
			domain.PageRange{Start: pageRange.Start, End: mid},
			domain.PageRange{Start: mid + 1, End: pageRange.End},
		)
		ranges = append(ranges, plan.Ranges[i+1:]...)
		result := domain.SplitPlan{Ranges: ranges}
		return result, result.Validate(totalPages)
	}
	return domain.SplitPlan{}, fmt.Errorf("range %d-%d is not present in plan", target.Start, target.End)
}

func appendUniqueRange(ranges []domain.PageRange, target domain.PageRange) []domain.PageRange {
	for _, pageRange := range ranges {
		if pageRange == target {
			return ranges
		}
	}
	return append(ranges, target)
}

func createPlan(ctx context.Context, opts Options, totalPages int, engine pdf.Engine, reporter progress.Reporter) (domain.SplitPlan, []int64, []domain.PageRange, error) {
	reporter.Planning(0)
	if opts.MaxSize > 0 {
		inputBytes, err := depsInputSize(engine, opts.Input)
		if err != nil {
			return domain.SplitPlan{}, nil, nil, fmt.Errorf("stat input PDF %q: %w", opts.Input, err)
		}
		plan, err := planner.ByEstimatedMaxSize(totalPages, inputBytes, opts.MaxSize, opts.Parts)
		return plan, nil, nil, err
	}

	tempDir, err := os.MkdirTemp("", "pdf-split-measure-*")
	if err != nil {
		return domain.SplitPlan{}, nil, nil, err
	}
	defer os.RemoveAll(tempDir)
	measurer := measure.NewWithProgress(engine, opts.Input, tempDir, 512, func(event measure.ProgressEvent) {
		if event.Done {
			reporter.Planning(event.Completed)
			return
		}
		reporter.PlanningRange(event.Range, event.Completed)
	})
	result, err := planner.ByBalancedParts(ctx, totalPages, opts.Parts, measurer, 64)
	return result.Plan, result.Sizes, nil, err
}

type inputSizer interface {
	InputSize(path string) (int64, error)
}

func depsInputSize(engine pdf.Engine, path string) (int64, error) {
	if sizer, ok := engine.(inputSizer); ok {
		return sizer.InputSize(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func withDefaults(deps Dependencies) Dependencies {
	if deps.Engine == nil {
		deps.Engine = pdf.NewPDFCPUEngine()
	}
	if deps.Begin == nil {
		deps.Begin = func(outputDir string, names []string, overwrite bool) (Transaction, error) {
			return output.Begin(outputDir, names, overwrite, nil)
		}
	}
	if deps.Verify == nil {
		deps.Verify = verify.Verify
	}
	if deps.Stdout == nil {
		deps.Stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	if deps.NewReporter == nil {
		deps.NewReporter = func(bool) progress.Reporter { return progress.New(io.Discard, false) }
	}
	return deps
}

func validateOptions(opts Options) error {
	if opts.Input == "" {
		return errors.New("input path is required")
	}
	if opts.Parts < 0 || opts.MaxSize < 0 || opts.Parts == 0 && opts.MaxSize == 0 {
		return errors.New("at least one valid split constraint is required")
	}
	if opts.OutputDir == "" {
		return errors.New("output directory is required")
	}
	return nil
}

func classifyInputError(err error) error {
	if errors.Is(err, pdf.ErrEncrypted) {
		return exit(4, err)
	}
	return exit(3, err)
}

func classifyPlanningError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return exit(8, err)
	}
	if errors.Is(err, pdf.ErrEncrypted) {
		return exit(4, err)
	}
	return exit(5, err)
}

func classifyWriteError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return exit(8, err)
	}
	if errors.Is(err, pdf.ErrEncrypted) {
		return exit(4, err)
	}
	return exit(6, err)
}

func exit(code int, err error) error {
	return &ExitError{Code: code, Err: err}
}

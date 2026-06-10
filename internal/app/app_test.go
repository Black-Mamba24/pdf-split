package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
	"github.com/Black-Mamba24/pdf-split/internal/progress"
	"github.com/Black-Mamba24/pdf-split/internal/verify"
)

func TestRunPartsOnlyGeneratesExactlyRequestedParts(t *testing.T) {
	engine := newFakeEngine(5, 10)
	outputDir := filepath.Join(t.TempDir(), "out")

	err := Run(context.Background(), Options{Input: "input.pdf", Parts: 3, OutputDir: outputDir}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{"input-001.pdf", "input-002.pdf", "input-003.pdf"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("missing output %q: %v", name, err)
		}
	}
}

func TestRunPartsOnlyBalancesMeasuredOutputSizes(t *testing.T) {
	engine := newFakeEngine(4, 0)
	engine.weights = []int64{50, 50, 1, 1}

	err := Run(context.Background(), Options{Input: "input.pdf", Parts: 2, OutputDir: t.TempDir()}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var generated []domain.PageRange
	for path, pageRange := range engine.outputs {
		if !strings.Contains(path, ".pdf-split-stage-") {
			continue
		}
		generated = append(generated, pageRange)
	}
	if !containsRange(generated, domain.PageRange{Start: 1, End: 1}) ||
		!containsRange(generated, domain.PageRange{Start: 2, End: 4}) {
		t.Fatalf("generated ranges = %#v, want measured-size-balanced ranges", generated)
	}
}

func TestRunCombinedUsesAtLeastRequestedParts(t *testing.T) {
	engine := newFakeEngine(6, 10)
	outputDir := t.TempDir()

	err := Run(context.Background(), Options{Input: "input.pdf", Parts: 4, MaxSize: 100, OutputDir: outputDir}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outputDir, "input-*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) < 4 {
		t.Fatalf("generated %d files, want at least 4", len(matches))
	}
}

func TestRunMaxSizeUsesMeasuredPageSizesAndPublishesToOutputDir(t *testing.T) {
	engine := newFakeEngine(5, 0)
	engine.weights = []int64{40, 40, 20, 80, 10}
	outputDir := t.TempDir()

	err := Run(context.Background(), Options{Input: "input.pdf", MaxSize: 100, OutputDir: outputDir}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(outputDir, "input-*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("generated %d files, want 2", len(matches))
	}
	if !containsRange(engine.publishedRanges(), domain.PageRange{Start: 1, End: 3}) ||
		!containsRange(engine.publishedRanges(), domain.PageRange{Start: 4, End: 5}) {
		t.Fatalf("published ranges = %#v, want measured max-size ranges", engine.publishedRanges())
	}
}

func TestRunMaxSizeReportsScanningProgress(t *testing.T) {
	engine := newFakeEngine(3, 10)
	var stderr bytes.Buffer
	deps := testDependencies(engine)
	deps.NewReporter = func(bool) progress.Reporter { return progress.NewWithTerminal(&stderr, true, false) }

	err := Run(context.Background(), Options{Input: "input.pdf", MaxSize: 100, OutputDir: t.TempDir()}, deps)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Scanning PDF pages: 3/3 measured") {
		t.Fatalf("stderr = %q, want scanning progress", stderr.String())
	}
}

func TestRunWarnsButSucceedsForOversizedSinglePage(t *testing.T) {
	engine := newFakeEngine(2, 10)
	engine.sizes[domain.PageRange{Start: 1, End: 1}] = 200
	engine.finalSizes[domain.PageRange{Start: 1, End: 2}] = 200
	engine.finalSizes[domain.PageRange{Start: 1, End: 1}] = 200
	var stderr bytes.Buffer
	deps := testDependencies(engine)
	deps.Stderr = &stderr

	err := Run(context.Background(), Options{Input: "input.pdf", MaxSize: 100, OutputDir: t.TempDir()}, deps)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("page 1")) {
		t.Fatalf("stderr = %q, want oversized page warning", stderr.String())
	}
}

func TestRunAbortsTransactionOnGenerationFailure(t *testing.T) {
	engine := newFakeEngine(3, 10)
	engine.writeErr = errors.New("write failed")
	outputDir := t.TempDir()

	err := Run(context.Background(), Options{Input: "input.pdf", Parts: 2, OutputDir: outputDir}, testDependencies(engine))
	if err == nil {
		t.Fatal("Run() error = nil, want generation failure")
	}
	matches, globErr := filepath.Glob(filepath.Join(outputDir, "input-*.pdf"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("published files after failure: %v", matches)
	}
}

func TestRunAbortsOnVerificationFailure(t *testing.T) {
	engine := newFakeEngine(3, 10)
	outputDir := t.TempDir()
	deps := testDependencies(engine)
	deps.Verify = func(verify.Inspector, verify.Request) error {
		return errors.New("verification failed")
	}

	err := Run(context.Background(), Options{Input: "input.pdf", Parts: 2, OutputDir: outputDir}, deps)
	if err == nil || ExitCode(err) != 7 {
		t.Fatalf("Run() error = %v, code = %d; want verification failure code 7", err, ExitCode(err))
	}
	matches, globErr := filepath.Glob(filepath.Join(outputDir, "input-*.pdf"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("published files after verification failure: %v", matches)
	}
}

func TestRunReplansWhenFinalGeneratedFileExceedsMeasuredLimit(t *testing.T) {
	engine := newFakeEngine(4, 10)
	engine.finalSizes[domain.PageRange{Start: 1, End: 4}] = 200
	outputDir := t.TempDir()

	err := Run(context.Background(), Options{Input: "input.pdf", MaxSize: 100, OutputDir: outputDir}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	matches, globErr := filepath.Glob(filepath.Join(outputDir, "input-*.pdf"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 2 {
		t.Fatalf("generated %d files, want 2 after replan", len(matches))
	}
}

func TestRunStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, Options{Input: "input.pdf", Parts: 2, OutputDir: t.TempDir()}, testDependencies(newFakeEngine(3, 10)))
	if !errors.Is(err, context.Canceled) || ExitCode(err) != 8 {
		t.Fatalf("Run() error = %v, code = %d; want context.Canceled and 8", err, ExitCode(err))
	}
}

func TestRunUsesAndClosesSessionAtOneGiB(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, maxReusableMeasurementInputBytes), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if err != nil {
		t.Fatal(err)
	}
	if engine.sessionOpens != 1 || engine.sessionMeasures == 0 || engine.sessionCloses != 1 {
		t.Fatalf("session opens=%d measures=%d closes=%d", engine.sessionOpens, engine.sessionMeasures, engine.sessionCloses)
	}
	for path := range engine.outputs {
		if strings.Contains(path, "pdf-split-measure-") {
			t.Fatalf("reusable session created measurement candidate %q", path)
		}
	}
}

func TestRunSkipsSessionAboveOneGiB(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, maxReusableMeasurementInputBytes+1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if err != nil {
		t.Fatal(err)
	}
	if engine.sessionOpens != 0 {
		t.Fatalf("session opens = %d, want 0", engine.sessionOpens)
	}
}

func TestRunFallsBackWhenSessionIsUnsupported(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionOpenErr: pdf.ErrMeasurementSessionUnsupported}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if err != nil {
		t.Fatal(err)
	}
	if engine.sessionOpens != 1 || engine.sessionMeasures != 0 {
		t.Fatalf("session opens=%d measures=%d", engine.sessionOpens, engine.sessionMeasures)
	}
}

func TestRunStopsOnUnexpectedSessionOpenFailure(t *testing.T) {
	wantErr := errors.New("open failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionOpenErr: wantErr}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, wantErr) || ExitCode(err) != 5 {
		t.Fatalf("Run() error = %v, code = %d; want %v and code 5", err, ExitCode(err), wantErr)
	}
}

func TestRunClosesSessionAfterPlanningFailure(t *testing.T) {
	wantErr := errors.New("measure failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionMeasureErr: wantErr}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, wantErr) || engine.sessionCloses != 1 {
		t.Fatalf("Run() error=%v closes=%d", err, engine.sessionCloses)
	}
}

func TestRunClosesSessionAfterPlanningCancellation(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionMeasureErr: context.Canceled}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, context.Canceled) || ExitCode(err) != 8 || engine.sessionCloses != 1 {
		t.Fatalf("Run() error=%v code=%d closes=%d", err, ExitCode(err), engine.sessionCloses)
	}
}

func TestRunReturnsSessionCloseErrorAfterSuccessfulPlanning(t *testing.T) {
	wantErr := errors.New("close failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionCloseErr: wantErr}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, wantErr) || ExitCode(err) != 5 {
		t.Fatalf("Run() error = %v, code = %d; want %v and code 5", err, ExitCode(err), wantErr)
	}
}

func TestRunKeepsPlanningErrorWhenSessionCloseAlsoFails(t *testing.T) {
	planErr := errors.New("measure failed")
	closeErr := errors.New("close failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionMeasureErr: planErr, sessionCloseErr: closeErr}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, planErr) || errors.Is(err, closeErr) {
		t.Fatalf("Run() error = %v, want only planning error %v", err, planErr)
	}
	if engine.sessionCloses != 1 {
		t.Fatalf("session closes = %d, want 1", engine.sessionCloses)
	}
}

func TestRunClassifiesSessionInputStatFailureAsInputError(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	err := Run(context.Background(), Options{
		Input: filepath.Join(t.TempDir(), "missing.pdf"), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if ExitCode(err) != 3 {
		t.Fatalf("Run() error = %v, code = %d; want input error code 3", err, ExitCode(err))
	}
	if engine.sessionOpens != 0 {
		t.Fatalf("session opens = %d, want 0", engine.sessionOpens)
	}
}

func TestRunReplansWhenSessionPlanningReferenceUnderestimatesFinalSize(t *testing.T) {
	initial := domain.PageRange{Start: 1, End: 4}
	engine := &sessionEngine{
		fakeEngine:   newFakeEngine(4, 10),
		sessionSizes: map[domain.PageRange]int64{initial: 40},
	}
	engine.finalSizes[initial] = 200

	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), MaxSize: 100, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !containsRange(engine.publishedRanges(), domain.PageRange{Start: 1, End: 2}) ||
		!containsRange(engine.publishedRanges(), domain.PageRange{Start: 3, End: 4}) {
		t.Fatalf("published ranges = %#v, want final verification replan", engine.publishedRanges())
	}
}

type fakeMeasurementSession struct {
	engine *sessionEngine
}

func (s *fakeMeasurementSession) MeasureRange(pageRange domain.PageRange) (int64, error) {
	s.engine.sessionMeasures++
	if s.engine.sessionMeasureErr != nil {
		return 0, s.engine.sessionMeasureErr
	}
	if size := s.engine.sessionSizes[pageRange]; size > 0 {
		return size, nil
	}
	return s.engine.fakeEngine.rangeSize(pageRange), nil
}

func (s *fakeMeasurementSession) Close() error {
	s.engine.sessionCloses++
	return s.engine.sessionCloseErr
}

type sessionEngine struct {
	*fakeEngine
	sessionOpenErr    error
	sessionMeasureErr error
	sessionCloseErr   error
	sessionSizes      map[domain.PageRange]int64
	sessionOpens      int
	sessionMeasures   int
	sessionCloses     int
}

func (e *sessionEngine) OpenMeasurementSession(string) (pdf.MeasurementSession, error) {
	e.sessionOpens++
	if e.sessionOpenErr != nil {
		return nil, e.sessionOpenErr
	}
	if e.sessionSizes == nil {
		e.sessionSizes = make(map[domain.PageRange]int64)
	}
	return &fakeMeasurementSession{engine: e}, nil
}

type fakeEngine struct {
	pages      int
	perPage    int64
	sizes      map[domain.PageRange]int64
	finalSizes map[domain.PageRange]int64
	weights    []int64
	outputs    map[string]domain.PageRange
	writeErr   error
}

func newFakeEngine(pages int, perPage int64) *fakeEngine {
	return &fakeEngine{
		pages: pages, perPage: perPage,
		sizes: make(map[domain.PageRange]int64), finalSizes: make(map[domain.PageRange]int64),
		outputs: make(map[string]domain.PageRange),
	}
}

func (e *fakeEngine) Inspect(path string) (pdf.Info, error) {
	if pageRange, ok := e.outputs[path]; ok {
		return pdf.Info{Pages: pageRange.Pages()}, nil
	}
	return pdf.Info{Pages: e.pages}, nil
}

func (e *fakeEngine) rangeSize(pageRange domain.PageRange) int64 {
	size := e.sizes[pageRange]
	if size != 0 {
		return size
	}
	if len(e.weights) > 0 {
		for page := pageRange.Start; page <= pageRange.End; page++ {
			size += e.weights[page-1]
		}
		return size
	}
	return int64(pageRange.Pages()) * e.perPage
}

func (e *fakeEngine) WriteRange(_ string, outputPath string, pageRange domain.PageRange) error {
	if e.writeErr != nil {
		return e.writeErr
	}
	size := e.rangeSize(pageRange)
	if finalSize := e.finalSizes[pageRange]; finalSize > 0 && !strings.Contains(outputPath, "pdf-split-measure-") {
		size = finalSize
	}
	if err := os.WriteFile(outputPath, make([]byte, size), 0o600); err != nil {
		return err
	}
	e.outputs[outputPath] = pageRange
	return nil
}

func (e *fakeEngine) publishedRanges() []domain.PageRange {
	var ranges []domain.PageRange
	for path, pageRange := range e.outputs {
		if strings.Contains(path, "pdf-split-measure-") {
			continue
		}
		ranges = append(ranges, pageRange)
	}
	return ranges
}

func containsRange(ranges []domain.PageRange, target domain.PageRange) bool {
	for _, pageRange := range ranges {
		if pageRange == target {
			return true
		}
	}
	return false
}

func sparseInput(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.pdf")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func testDependencies(engine pdf.Engine) Dependencies {
	return Dependencies{Engine: engine, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

# Reusable PDF Measurement Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce PDF split planning time by parsing eligible inputs once and measuring candidate ranges without temporary files, while preserving the existing low-memory path for inputs larger than 1 GiB.

**Architecture:** Add an optional `pdf.MeasurementSessionOpener` capability implemented by the pdfcpu engine. `internal/app` selects this capability only for inputs at most 1 GiB, owns the session lifetime, and injects it into the existing cached measurer; all final output generation and verification continue through `Engine.WriteRange`.

**Tech Stack:** Go 1.25, pdfcpu v0.12.1, standard `testing`, existing planner/measure/app packages.

---

## File Structure

- Modify `internal/pdf/engine.go`: define the optional session interfaces and fallback sentinel.
- Create `internal/pdf/measurement_session.go`: implement pdfcpu session lifecycle, page extraction, and byte counting.
- Create `internal/pdf/measurement_session_test.go`: verify exact sizes, stable reuse, bounds, and close behavior.
- Modify `internal/measure/measurer.go`: allow cache misses to measure through an optional session.
- Modify `internal/measure/measurer_test.go`: verify session-backed caching, progress, cancellation, and error propagation.
- Modify `internal/app/app.go`: select the strategy at 1 GiB and close the session after planning.
- Modify `internal/app/app_test.go`: verify threshold, fallback, error, and close behavior.
- Modify `internal/integration/scale_test.go`: add an opt-in comparative planning benchmark.

## Task 1: Add the pdfcpu Reusable Measurement Session

**Files:**
- Modify: `internal/pdf/engine.go`
- Create: `internal/pdf/measurement_session.go`
- Create: `internal/pdf/measurement_session_test.go`

- [ ] **Step 1: Add failing interface and exact-size session tests**

Create `internal/pdf/measurement_session_test.go` with tests that compare the
session byte count with the existing file-backed output and exercise repeated
reuse:

```go
package pdf

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestPDFCPUMeasurementSessionMatchesWriteRangeSize(t *testing.T) {
	engine := NewPDFCPUEngine()
	opener, ok := engine.(MeasurementSessionOpener)
	if !ok {
		t.Fatal("pdfcpu engine does not implement MeasurementSessionOpener")
	}
	session, err := opener.OpenMeasurementSession(filepath.Join("testdata", "basic.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for _, pages := range []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
		{Start: 1, End: 5},
	} {
		got, err := session.MeasureRange(pages)
		if err != nil {
			t.Fatalf("MeasureRange(%+v): %v", pages, err)
		}
		output := filepath.Join(t.TempDir(), "range.pdf")
		if err := engine.WriteRange(filepath.Join("testdata", "basic.pdf"), output, pages); err != nil {
			t.Fatalf("WriteRange(%+v): %v", pages, err)
		}
		info, err := os.Stat(output)
		if err != nil {
			t.Fatal(err)
		}
		if got != info.Size() {
			t.Fatalf("MeasureRange(%+v) = %d, WriteRange size = %d", pages, got, info.Size())
		}
	}
}

func TestPDFCPUMeasurementSessionRemainsStableAcrossOverlappingRanges(t *testing.T) {
	opener := NewPDFCPUEngine().(MeasurementSessionOpener)
	session, err := opener.OpenMeasurementSession(filepath.Join("testdata", "basic.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	rangeA := domain.PageRange{Start: 1, End: 4}
	first, err := session.MeasureRange(rangeA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.MeasureRange(domain.PageRange{Start: 2, End: 5}); err != nil {
		t.Fatal(err)
	}
	second, err := session.MeasureRange(rangeA)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("repeated size = %d, want %d", second, first)
	}
}

func TestPDFCPUMeasurementSessionRejectsInvalidRangesAndUseAfterClose(t *testing.T) {
	opener := NewPDFCPUEngine().(MeasurementSessionOpener)
	session, err := opener.OpenMeasurementSession(filepath.Join("testdata", "basic.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pages := range []domain.PageRange{
		{Start: 0, End: 1},
		{Start: 2, End: 1},
		{Start: 1, End: 6},
	} {
		if _, err := session.MeasureRange(pages); err == nil {
			t.Fatalf("MeasureRange(%+v) unexpectedly succeeded", pages)
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.MeasureRange(domain.PageRange{Start: 1, End: 1}); !errors.Is(err, ErrMeasurementSessionClosed) {
		t.Fatalf("MeasureRange after Close error = %v, want ErrMeasurementSessionClosed", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
```

- [ ] **Step 2: Run the pdf package tests to verify failure**

Run:

```bash
go test ./internal/pdf -run 'TestPDFCPUMeasurementSession' -count=1
```

Expected: compilation fails because `MeasurementSessionOpener` and
`ErrMeasurementSessionClosed` are undefined.

- [ ] **Step 3: Define optional session interfaces and sentinels**

Modify `internal/pdf/engine.go`:

```go
var (
	ErrEncrypted                     = errors.New("encrypted PDF is not supported")
	ErrInvalid                       = errors.New("invalid PDF")
	ErrMeasurementSessionUnsupported = errors.New("measurement session is unsupported")
	ErrMeasurementSessionClosed      = errors.New("measurement session is closed")
)

type MeasurementSession interface {
	MeasureRange(pageRange domain.PageRange) (int64, error)
	Close() error
}

type MeasurementSessionOpener interface {
	OpenMeasurementSession(inputPath string) (MeasurementSession, error)
}
```

Do not add these methods to `Engine`; engines without this optimization must
continue to compile and use the existing path.

- [ ] **Step 4: Implement the pdfcpu session and counting writer**

Create `internal/pdf/measurement_session.go`:

```go
package pdf

import (
	"fmt"
	"os"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type pdfcpuMeasurementSession struct {
	file   *os.File
	source *model.Context
	pages  int
	closed bool
}

type countingWriter struct {
	bytes int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.bytes += int64(len(p))
	return len(p), nil
}

func (e *pdfcpuEngine) OpenMeasurementSession(inputPath string) (MeasurementSession, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open measurement session for %q: %w", inputPath, err)
	}
	conf := fastConfiguration()
	conf.Cmd = model.TRIM
	source, err := api.ReadValidateAndOptimize(file, conf)
	if err != nil {
		_ = file.Close()
		return nil, classifyInspectError(inputPath, err)
	}
	e.storePages(inputPath, source.PageCount)
	return &pdfcpuMeasurementSession{file: file, source: source, pages: source.PageCount}, nil
}

func (s *pdfcpuMeasurementSession) MeasureRange(pageRange domain.PageRange) (int64, error) {
	if s.closed {
		return 0, ErrMeasurementSessionClosed
	}
	if pageRange.Start < 1 || pageRange.End < pageRange.Start || pageRange.End > s.pages {
		return 0, fmt.Errorf("page range %d-%d is outside page bounds 1-%d", pageRange.Start, pageRange.End, s.pages)
	}
	pageNumbers := make([]int, 0, pageRange.Pages())
	for page := pageRange.Start; page <= pageRange.End; page++ {
		pageNumbers = append(pageNumbers, page)
	}
	candidate, err := pdfcpu.ExtractPages(s.source, pageNumbers, false)
	if err != nil {
		return 0, fmt.Errorf("extract measurement range %d-%d: %w", pageRange.Start, pageRange.End, err)
	}
	writer := &countingWriter{}
	if err := api.WriteContext(candidate, writer); err != nil {
		return 0, fmt.Errorf("write measurement range %d-%d: %w", pageRange.Start, pageRange.End, err)
	}
	return writer.bytes, nil
}

func (s *pdfcpuMeasurementSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.source = nil
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close measurement session: %w", err)
	}
	s.file = nil
	return nil
}
```

Keep the session serial. Do not add locks or goroutines.

- [ ] **Step 5: Run focused and package tests**

Run:

```bash
gofmt -w internal/pdf/engine.go internal/pdf/measurement_session.go internal/pdf/measurement_session_test.go
go test ./internal/pdf -count=1
```

Expected: all `internal/pdf` tests pass, including exact byte equality.

- [ ] **Step 6: Commit the pdf session**

```bash
git add internal/pdf/engine.go internal/pdf/measurement_session.go internal/pdf/measurement_session_test.go
git commit -m "feat: add reusable PDF measurement session"
```

## Task 2: Let the Cached Measurer Use a Session

**Files:**
- Modify: `internal/measure/measurer.go`
- Modify: `internal/measure/measurer_test.go`

- [ ] **Step 1: Add failing session-backed measurer tests**

Append a fake session and focused tests to `internal/measure/measurer_test.go`:

```go
func TestMeasureWithSessionCachesAndReportsProgress(t *testing.T) {
	pages := domain.PageRange{Start: 2, End: 4}
	session := &fakeSession{sizes: map[domain.PageRange]int64{pages: 123}}
	var events []ProgressEvent
	measurer := NewWithSession(session, 8, func(event ProgressEvent) {
		events = append(events, event)
	})

	for i := 0; i < 2; i++ {
		got, err := measurer.Measure(context.Background(), pages)
		if err != nil {
			t.Fatal(err)
		}
		if got != 123 {
			t.Fatalf("Measure() = %d, want 123", got)
		}
	}
	if session.calls != 1 {
		t.Fatalf("MeasureRange calls = %d, want 1", session.calls)
	}
	if measurer.Measurements() != 1 {
		t.Fatalf("Measurements() = %d, want 1", measurer.Measurements())
	}
	if len(events) != 2 || events[0].Done || !events[1].Done {
		t.Fatalf("events = %#v, want one start and one completion", events)
	}
}

func TestMeasureWithSessionChecksCancellationBeforeMeasuring(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session := &fakeSession{}
	measurer := NewWithSession(session, 8, nil)

	_, err := measurer.Measure(ctx, domain.PageRange{Start: 1, End: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Measure() error = %v, want context.Canceled", err)
	}
	if session.calls != 0 {
		t.Fatalf("MeasureRange calls = %d, want 0", session.calls)
	}
}

func TestMeasureWithSessionPropagatesFailureWithoutCompletion(t *testing.T) {
	wantErr := errors.New("measure failed")
	session := &fakeSession{err: wantErr}
	var events []ProgressEvent
	measurer := NewWithSession(session, 8, func(event ProgressEvent) {
		events = append(events, event)
	})

	_, err := measurer.Measure(context.Background(), domain.PageRange{Start: 1, End: 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Measure() error = %v, want %v", err, wantErr)
	}
	if len(events) != 1 || events[0].Done {
		t.Fatalf("events = %#v, want only start event", events)
	}
}

type fakeSession struct {
	sizes map[domain.PageRange]int64
	calls int
	err   error
}

func (s *fakeSession) MeasureRange(pageRange domain.PageRange) (int64, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return s.sizes[pageRange], nil
}

func (s *fakeSession) Close() error { return nil }
```

- [ ] **Step 2: Run focused tests to verify failure**

Run:

```bash
go test ./internal/measure -run 'TestMeasureWithSession' -count=1
```

Expected: compilation fails because `NewWithSession` is undefined.

- [ ] **Step 3: Add a session field and constructor**

Modify `internal/measure/measurer.go`:

```go
type measurer struct {
	engine         pdf.Engine
	session        pdf.MeasurementSession
	input          string
	canonicalInput string
	tempDir        string
	cacheEntries   int
	onProgress     func(ProgressEvent)
	// existing mutex/cache fields remain unchanged
}

func NewWithSession(session pdf.MeasurementSession, cacheEntries int, onProgress func(ProgressEvent)) Measurer {
	return &measurer{
		session:      session,
		cacheEntries: cacheEntries,
		onProgress:   onProgress,
		lru:          list.New(),
		cache:        make(map[cacheKey]*list.Element),
	}
}
```

The session-backed cache key can use an empty `input` because one measurer owns
exactly one session.

- [ ] **Step 4: Route cache misses through the selected backend**

Replace the file-writing block in `Measure` with a small backend helper:

```go
	m.reportProgress(pages, false)
	size, err := m.measureUncached(pages)
	if err != nil {
		return 0, err
	}
	m.store(key, size)
	m.recordMeasurement()
	m.reportProgress(pages, true)
	return size, nil
}

func (m *measurer) measureUncached(pages domain.PageRange) (int64, error) {
	if m.session != nil {
		return m.session.MeasureRange(pages)
	}
	candidate, err := os.CreateTemp(m.tempDir, "pdf-split-measure-*.pdf")
	if err != nil {
		return 0, fmt.Errorf("create measurement candidate: %w", err)
	}
	candidatePath := candidate.Name()
	if err := candidate.Close(); err != nil {
		_ = os.Remove(candidatePath)
		return 0, fmt.Errorf("close measurement candidate: %w", err)
	}
	if err := os.Remove(candidatePath); err != nil {
		return 0, fmt.Errorf("prepare measurement candidate: %w", err)
	}
	defer os.Remove(candidatePath)
	if err := m.engine.WriteRange(m.input, candidatePath, pages); err != nil {
		return 0, err
	}
	info, err := os.Stat(candidatePath)
	if err != nil {
		return 0, fmt.Errorf("stat measurement candidate: %w", err)
	}
	return info.Size(), nil
}
```

Do not move cache, progress, or measurement counting into the backend helper.

- [ ] **Step 5: Run measurer and full unit tests**

Run:

```bash
gofmt -w internal/measure/measurer.go internal/measure/measurer_test.go
go test ./internal/measure -count=1
go test ./internal/pdf ./internal/planner ./internal/app -count=1
```

Expected: all commands pass; existing file-backed cleanup tests remain green.

- [ ] **Step 6: Commit the measurer integration**

```bash
git add internal/measure/measurer.go internal/measure/measurer_test.go
git commit -m "feat: measure candidate ranges through reusable sessions"
```

## Task 3: Select the Session at the 1 GiB Threshold

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Add failing app strategy-selection tests**

Extend the app-test fake engine with optional session behavior:

```go
type fakeMeasurementSession struct {
	engine   *sessionEngine
	closed   bool
	closeErr error
}

func (s *fakeMeasurementSession) MeasureRange(pageRange domain.PageRange) (int64, error) {
	s.engine.sessionMeasures++
	if s.engine.sessionMeasureErr != nil {
		return 0, s.engine.sessionMeasureErr
	}
	return s.engine.fakeEngine.rangeSize(pageRange), nil
}

func (s *fakeMeasurementSession) Close() error {
	s.closed = true
	s.engine.sessionCloses++
	return s.closeErr
}

type sessionEngine struct {
	*fakeEngine
	sessionOpenErr    error
	sessionMeasureErr error
	sessionOpens      int
	sessionMeasures   int
	sessionCloses     int
	openSession       func() pdf.MeasurementSession
}

func (e *sessionEngine) OpenMeasurementSession(string) (pdf.MeasurementSession, error) {
	e.sessionOpens++
	if e.sessionOpenErr != nil {
		return nil, e.sessionOpenErr
	}
	if e.openSession != nil {
		return e.openSession(), nil
	}
	return &fakeMeasurementSession{engine: e}, nil
}
```

Move the fake range-size calculation from `WriteRange` into:

```go
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
```

Replace the size-selection portion of `fakeEngine.WriteRange` so final-output
overrides remain unchanged while planning measurements share `rangeSize`:

```go
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
```

Add threshold and lifecycle tests using sparse input files:

```go
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

func TestRunUsesAndClosesSessionAtOneGiB(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, maxReusableMeasurementInputBytes),
		Parts: 2, OutputDir: t.TempDir(),
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
		Input: sparseInput(t, maxReusableMeasurementInputBytes+1),
		Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if err != nil {
		t.Fatal(err)
	}
	if engine.sessionOpens != 0 {
		t.Fatalf("session opens = %d, want 0", engine.sessionOpens)
	}
}

func TestRunFallsBackWhenSessionIsUnsupported(t *testing.T) {
	engine := &sessionEngine{
		fakeEngine:      newFakeEngine(3, 10),
		sessionOpenErr: pdf.ErrMeasurementSessionUnsupported,
	}
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
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
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

func TestRunClassifiesSessionInputStatFailureAsInputError(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	err := Run(context.Background(), Options{
		Input: filepath.Join(t.TempDir(), "missing.pdf"),
		Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if ExitCode(err) != 3 {
		t.Fatalf("Run() error = %v, code = %d; want input error code 3", err, ExitCode(err))
	}
	if engine.sessionOpens != 0 {
		t.Fatalf("session opens = %d, want 0", engine.sessionOpens)
	}
}
```

- [ ] **Step 2: Run focused app tests to verify failure**

Run:

```bash
go test ./internal/app -run 'TestRun(UsesAndClosesSession|SkipsSession|FallsBackWhenSession|StopsOnUnexpectedSession|ClosesSession)' -count=1
```

Expected: compilation fails because `maxReusableMeasurementInputBytes` and
session selection are undefined.

- [ ] **Step 3: Add strategy selection and session lifetime management**

Modify `internal/app/app.go`:

```go
const maxReusableMeasurementInputBytes int64 = 1 << 30

var errInputUnavailable = errors.New("input is unavailable")
```

Change `createPlan` to select and close the session. Use named return values so
a close error is returned only when planning has not already failed:

```go
func createPlan(ctx context.Context, opts Options, totalPages int, engine pdf.Engine, reporter progress.Reporter) (
	plan domain.SplitPlan,
	sizes []int64,
	oversized []domain.PageRange,
	err error,
) {
	reporter.Planning(0)
	tempDir, err := os.MkdirTemp("", "pdf-split-measure-*")
	if err != nil {
		return domain.SplitPlan{}, nil, nil, err
	}
	defer os.RemoveAll(tempDir)

	onProgress := func(event measure.ProgressEvent) {
		if opts.MaxSize > 0 && event.Range.Pages() == 1 {
			if event.Done {
				reporter.ScanningPages(event.Range.End, totalPages)
			}
			return
		}
		if event.Done {
			reporter.Planning(event.Completed)
			return
		}
		reporter.PlanningRange(event.Range, event.Completed)
	}

	measurer := measure.NewWithProgress(engine, opts.Input, tempDir, 512, onProgress)
	if opener, ok := engine.(pdf.MeasurementSessionOpener); ok {
		info, statErr := os.Stat(opts.Input)
		if statErr != nil {
			return domain.SplitPlan{}, nil, nil, fmt.Errorf("%w: stat input %q: %v", errInputUnavailable, opts.Input, statErr)
		}
		if info.Size() <= maxReusableMeasurementInputBytes {
			session, openErr := opener.OpenMeasurementSession(opts.Input)
			switch {
			case openErr == nil:
				defer func() {
					if closeErr := session.Close(); err == nil && closeErr != nil {
						err = closeErr
					}
				}()
				measurer = measure.NewWithSession(session, 512, onProgress)
			case errors.Is(openErr, pdf.ErrMeasurementSessionUnsupported):
				// Keep the file-backed measurer.
			default:
				return domain.SplitPlan{}, nil, nil, openErr
			}
		}
	}

	if opts.MaxSize > 0 {
		result, planErr := planner.ByMaxSize(ctx, totalPages, measurer, planner.SizeOptions{
			MaxBytes:        opts.MaxSize,
			MinimumParts:    opts.Parts,
			LinearScan:      8,
			MaxMeasurements: maxSizeMeasurementBudget(totalPages),
		})
		if planErr != nil && !(errors.Is(planErr, planner.ErrMeasurementBudget) && result.Plan.Validate(totalPages) == nil) {
			return result.Plan, result.Sizes, result.OversizedSingles, planErr
		}
		return result.Plan, result.Sizes, result.OversizedSingles, nil
	}
	result, planErr := planner.ByBalancedParts(ctx, totalPages, opts.Parts, measurer, 64)
	return result.Plan, result.Sizes, nil, planErr
}
```

Important: check `MeasurementSessionOpener` before calling `os.Stat`. Existing
alternate/fake engines that do not expose the optional capability must continue
to work with virtual input paths such as `"input.pdf"`.

Update `classifyPlanningError` so the new stat failure is an input error:

```go
func classifyPlanningError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return exit(8, err)
	}
	if errors.Is(err, pdf.ErrEncrypted) {
		return exit(4, err)
	}
	if errors.Is(err, errInputUnavailable) {
		return exit(3, err)
	}
	return exit(5, err)
}
```

- [ ] **Step 4: Add a close-error precedence test**

Add one test proving a close error is returned after successful planning, while
a planning error remains primary:

```go
func TestRunReturnsSessionCloseErrorAfterSuccessfulPlanning(t *testing.T) {
	wantErr := errors.New("close failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10)}
	engine.openSession = func() pdf.MeasurementSession {
		return &fakeMeasurementSession{engine: engine, closeErr: wantErr}
	}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
}

func TestRunKeepsPlanningErrorWhenSessionCloseAlsoFails(t *testing.T) {
	planErr := errors.New("measure failed")
	closeErr := errors.New("close failed")
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionMeasureErr: planErr}
	engine.openSession = func() pdf.MeasurementSession {
		return &fakeMeasurementSession{engine: engine, closeErr: closeErr}
	}
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

func TestRunClosesSessionAfterPlanningCancellation(t *testing.T) {
	engine := &sessionEngine{fakeEngine: newFakeEngine(3, 10), sessionMeasureErr: context.Canceled}
	err := Run(context.Background(), Options{
		Input: sparseInput(t, 1), Parts: 2, OutputDir: t.TempDir(),
	}, testDependencies(engine))
	if !errors.Is(err, context.Canceled) || ExitCode(err) != 8 || engine.sessionCloses != 1 {
		t.Fatalf("Run() error=%v code=%d closes=%d", err, ExitCode(err), engine.sessionCloses)
	}
}
```

Add the `openSession func() pdf.MeasurementSession` hook to `sessionEngine` and
use it from `OpenMeasurementSession`. Keep it test-only.

- [ ] **Step 5: Run app and full tests**

Run:

```bash
gofmt -w internal/app/app.go internal/app/app_test.go
go test ./internal/app -count=1
go test ./... -count=1
```

Expected: all tests pass. Verify that engines without
`MeasurementSessionOpener` still use the file-backed path.

- [ ] **Step 6: Commit strategy selection**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat: reuse PDF measurement sessions for eligible inputs"
```

## Task 4: Add Comparative Integration Coverage and Benchmark

**Files:**
- Modify: `internal/integration/scale_test.go`

- [ ] **Step 1: Add a benchmark that runs both measurement strategies**

Add package imports for `context`, `errors`, `reflect`, `time`,
`internal/measure`, and `internal/planner`. Add a wrapper that deliberately
exposes only `pdf.Engine`, forcing the file-backed strategy:

```go
type fileBackedEngine struct {
	pdf.Engine
}

func BenchmarkPlanningStrategies(b *testing.B) {
	input := os.Getenv("PDF_SPLIT_BENCH_INPUT")
	if input == "" {
		b.Skip("set PDF_SPLIT_BENCH_INPUT to a representative PDF")
	}
	engine := pdf.NewPDFCPUEngine()
	info, err := engine.Inspect(input)
	if err != nil {
		b.Fatal(err)
	}
	inputInfo, err := os.Stat(input)
	if err != nil {
		b.Fatal(err)
	}
	maxBytes := inputInfo.Size() / 4
	if maxBytes < 1 {
		maxBytes = 1
	}

	bench := func(b *testing.B, measurerFactory func(string) (measure.Measurer, func(), error)) {
		for i := 0; i < b.N; i++ {
			tempDir := b.TempDir()
			measurer, closeSession, err := measurerFactory(tempDir)
			if err != nil {
				b.Fatal(err)
			}
			start := time.Now()
			result, err := planner.ByMaxSize(context.Background(), info.Pages, measurer, planner.SizeOptions{
				MaxBytes: maxBytes, LinearScan: 8, MaxMeasurements: info.Pages*16 + 128,
			})
			closeSession()
			if err != nil && !errors.Is(err, planner.ErrMeasurementBudget) {
				b.Fatal(err)
			}
			if err := result.Plan.Validate(info.Pages); err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(measurer.Measurements()), "measurements/op")
			b.ReportMetric(float64(time.Since(start).Milliseconds()), "planning-ms/op")
		}
	}

	b.Run("file-backed", func(b *testing.B) {
		bench(b, func(tempDir string) (measure.Measurer, func(), error) {
			return measure.New(fileBackedEngine{engine}, input, tempDir, 512), func() {}, nil
		})
	})
	b.Run("reusable-session", func(b *testing.B) {
		opener, ok := engine.(pdf.MeasurementSessionOpener)
		if !ok {
			b.Fatal("pdfcpu engine does not support measurement sessions")
		}
		bench(b, func(string) (measure.Measurer, func(), error) {
			session, err := opener.OpenMeasurementSession(input)
			if err != nil {
				return nil, func() {}, err
			}
			return measure.NewWithSession(session, 512, nil), func() {
				if err := session.Close(); err != nil {
					b.Error(err)
				}
			}, nil
		})
	})
}
```

The benchmark must compile with no external fixture configured.

- [ ] **Step 2: Add a real-session integration equivalence test**

Add a test that compares reusable and file-backed measurements for the checked
in fixture:

```go
func TestMeasurementStrategiesProduceEquivalentPlan(t *testing.T) {
	input := filepath.Join("..", "pdf", "testdata", "basic.pdf")
	engine := pdf.NewPDFCPUEngine()
	info, err := engine.Inspect(input)
	if err != nil {
		t.Fatal(err)
	}
	fileMeasurer := measure.New(fileBackedEngine{engine}, input, t.TempDir(), 512)
	fileResult, err := planner.ByMaxSize(context.Background(), info.Pages, fileMeasurer, planner.SizeOptions{
		MaxBytes: 1800, LinearScan: 8, MaxMeasurements: info.Pages*16 + 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := engine.(pdf.MeasurementSessionOpener).OpenMeasurementSession(input)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	sessionResult, err := planner.ByMaxSize(context.Background(), info.Pages, measure.NewWithSession(session, 512, nil), planner.SizeOptions{
		MaxBytes: 1800, LinearScan: 8, MaxMeasurements: info.Pages*16 + 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fileResult, sessionResult) {
		t.Fatalf("file-backed = %#v, reusable = %#v", fileResult, sessionResult)
	}
}
```

The checked-in fixture has single-page outputs around 1050 bytes, so a
`MaxBytes` value of 1800 exercises multi-page candidate measurements while
requiring more than one planned range.

- [ ] **Step 3: Run integration tests and verify the benchmark compiles**

Run:

```bash
gofmt -w internal/integration/scale_test.go
go test ./internal/integration -count=1
go test ./internal/integration -run '^$' -bench BenchmarkPlanningStrategies -benchtime=1x
```

Expected: integration tests pass; the benchmark reports `SKIP` when
`PDF_SPLIT_BENCH_INPUT` is unset.

- [ ] **Step 4: Run the benchmark against a representative PDF**

Run:

```bash
PDF_SPLIT_BENCH_INPUT=/absolute/path/to/representative.pdf \
go test ./internal/integration -run '^$' -bench BenchmarkPlanningStrategies -benchmem -benchtime=3x
```

Expected:

- `reusable-session` planning time is at least 50% lower than `file-backed`.
- Both strategies report the same measurement count.
- Peak memory is separately observed with the platform process monitor and
  remains below `3 * input size + 512 MiB`.

Record the command output in the implementation summary; do not commit local
fixture paths or benchmark output.

- [ ] **Step 5: Commit integration coverage**

```bash
git add internal/integration/scale_test.go
git commit -m "test: compare PDF planning measurement strategies"
```

## Task 5: Final Verification

**Files:**
- Verify only; modify code only if a failing check exposes a defect covered by
  this design.

- [ ] **Step 1: Run formatting and static checks**

```bash
gofmt -w internal/pdf/engine.go internal/pdf/measurement_session.go internal/pdf/measurement_session_test.go internal/measure/measurer.go internal/measure/measurer_test.go internal/app/app.go internal/app/app_test.go internal/integration/scale_test.go
go vet ./...
```

Expected: no output from `gofmt`; `go vet ./...` passes.

- [ ] **Step 2: Run all tests without cache**

```bash
go test ./... -count=1
```

Expected: all tests pass. Environment-gated visual, scale, and benchmark tests
may report `SKIP`.

- [ ] **Step 3: Run race-sensitive session tests**

```bash
go test -race ./internal/pdf ./internal/measure ./internal/app -count=1
```

Expected: all tests pass with no race reports. This does not authorize
concurrent `MeasureRange` calls; it verifies the implemented serial path.

- [ ] **Step 4: Verify repository cleanliness and commit any test-only fixes**

```bash
git status --short
git diff --check
```

Expected: no uncommitted implementation changes and no whitespace errors. If a
covered defect required a final fix, commit it with a narrowly scoped message
before reporting completion.

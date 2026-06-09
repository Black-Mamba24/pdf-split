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

func TestRunWarnsButSucceedsForOversizedSinglePage(t *testing.T) {
	engine := newFakeEngine(2, 10)
	engine.sizes[domain.PageRange{Start: 1, End: 1}] = 200
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

type fakeEngine struct {
	pages      int
	perPage    int64
	sizes      map[domain.PageRange]int64
	finalSizes map[domain.PageRange]int64
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

func (e *fakeEngine) WriteRange(_ string, outputPath string, pageRange domain.PageRange) error {
	if e.writeErr != nil {
		return e.writeErr
	}
	size := e.sizes[pageRange]
	if finalSize := e.finalSizes[pageRange]; finalSize > 0 && !strings.Contains(outputPath, "pdf-split-measure-") {
		size = finalSize
	}
	if size == 0 {
		size = int64(pageRange.Pages()) * e.perPage
	}
	if err := os.WriteFile(outputPath, make([]byte, size), 0o600); err != nil {
		return err
	}
	e.outputs[outputPath] = pageRange
	return nil
}

func testDependencies(engine pdf.Engine) Dependencies {
	return Dependencies{Engine: engine, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

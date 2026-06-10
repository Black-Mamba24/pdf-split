package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Black-Mamba24/pdf-split/internal/measure"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
	"github.com/Black-Mamba24/pdf-split/internal/planner"
)

type fileBackedEngine struct {
	pdf.Engine
}

func TestScaleInput(t *testing.T) {
	input := os.Getenv("PDF_SPLIT_SCALE_INPUT")
	if input == "" {
		t.Skip("set PDF_SPLIT_SCALE_INPUT to a large PDF")
	}
	outputDir := t.TempDir()
	result := runCLI(t, input, "--max-size", "100MB", "--output", outputDir)
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	matches, err := filepath.Glob(filepath.Join(outputDir, "*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("scale run produced no outputs")
	}
	engine := pdf.NewPDFCPUEngine()
	for _, path := range matches {
		if _, err := engine.Inspect(path); err != nil {
			t.Fatalf("inspect %q: %v", path, err)
		}
	}
}

func TestMeasurementStrategiesProduceValidPlans(t *testing.T) {
	input := filepath.Join("..", "pdf", "testdata", "basic.pdf")
	engine := pdf.NewPDFCPUEngine()
	info, err := engine.Inspect(input)
	if err != nil {
		t.Fatal(err)
	}
	opts := planner.SizeOptions{MaxBytes: 1800, LinearScan: 8, MaxMeasurements: info.Pages*16 + 128}

	fileResult, err := planner.ByMaxSize(
		context.Background(),
		info.Pages,
		measure.New(fileBackedEngine{engine}, input, t.TempDir(), 512),
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileResult.Plan.Validate(info.Pages); err != nil {
		t.Fatalf("file-backed plan invalid: %v", err)
	}

	session, err := engine.(pdf.MeasurementSessionOpener).OpenMeasurementSession(input)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	sessionResult, err := planner.ByMaxSize(
		context.Background(),
		info.Pages,
		measure.NewWithSession(session, 512, nil),
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessionResult.Plan.Validate(info.Pages); err != nil {
		t.Fatalf("reusable-session plan invalid: %v", err)
	}
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

	run := func(b *testing.B, factory func(string) (measure.Measurer, func() error, error)) {
		b.Helper()
		for i := 0; i < b.N; i++ {
			tempDir := b.TempDir()
			start := time.Now()
			measurer, cleanup, err := factory(tempDir)
			if err != nil {
				b.Fatal(err)
			}
			result, planErr := planner.ByMaxSize(context.Background(), info.Pages, measurer, planner.SizeOptions{
				MaxBytes: maxBytes, LinearScan: 8, MaxMeasurements: info.Pages*16 + 128,
			})
			if closeErr := cleanup(); planErr == nil && closeErr != nil {
				b.Fatal(closeErr)
			}
			if planErr != nil && !errors.Is(planErr, planner.ErrMeasurementBudget) {
				b.Fatal(planErr)
			}
			if err := result.Plan.Validate(info.Pages); err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(measurer.Measurements()), "measurements/op")
			b.ReportMetric(float64(time.Since(start).Milliseconds()), "planning-ms/op")
		}
	}

	b.Run("file-backed", func(b *testing.B) {
		run(b, func(tempDir string) (measure.Measurer, func() error, error) {
			return measure.New(fileBackedEngine{engine}, input, tempDir, 512), func() error { return nil }, nil
		})
	})
	b.Run("reusable-session", func(b *testing.B) {
		opener := engine.(pdf.MeasurementSessionOpener)
		run(b, func(string) (measure.Measurer, func() error, error) {
			session, err := opener.OpenMeasurementSession(input)
			if err != nil {
				return nil, nil, err
			}
			return measure.NewWithSession(session, 512, nil), session.Close, nil
		})
	})
}

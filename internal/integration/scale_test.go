package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

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

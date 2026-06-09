package pdf

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

func TestPDFCPUEngineInspectBasic(t *testing.T) {
	info, err := NewPDFCPUEngine().Inspect(filepath.Join("testdata", "basic.pdf"))
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info.Pages != 5 || info.Encrypted {
		t.Fatalf("Inspect() = %#v, want 5 unencrypted pages", info)
	}
}

func TestPDFCPUEngineRejectsEncrypted(t *testing.T) {
	_, err := NewPDFCPUEngine().Inspect(filepath.Join("testdata", "encrypted.pdf"))
	if !errors.Is(err, ErrEncrypted) {
		t.Fatalf("Inspect() error = %v, want ErrEncrypted", err)
	}
}

func TestPDFCPUEngineClassifiesInvalidPDF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(path, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewPDFCPUEngine().Inspect(path)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Inspect() error = %v, want ErrInvalid", err)
	}
}

func TestPDFCPUEnginePreservesFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.pdf")

	_, err := NewPDFCPUEngine().Inspect(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Inspect() error = %v, want os.ErrNotExist", err)
	}
	if errors.Is(err, ErrInvalid) {
		t.Fatalf("Inspect() error = %v, must not classify a missing file as ErrInvalid", err)
	}
}

func TestPDFCPUEngineWriteRange(t *testing.T) {
	engine := NewPDFCPUEngine()
	output := filepath.Join(t.TempDir(), "pages-2-4.pdf")

	if err := engine.WriteRange(filepath.Join("testdata", "basic.pdf"), output, domain.PageRange{Start: 2, End: 4}); err != nil {
		t.Fatalf("WriteRange() error = %v", err)
	}

	info, err := engine.Inspect(output)
	if err != nil {
		t.Fatalf("Inspect(output) error = %v", err)
	}
	if info.Pages != 3 {
		t.Fatalf("Inspect(output) pages = %d, want 3", info.Pages)
	}
}

func TestPDFCPUEngineWriteRangeRejectsOutOfBoundsRange(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out.pdf")

	err := NewPDFCPUEngine().WriteRange(
		filepath.Join("testdata", "basic.pdf"),
		output,
		domain.PageRange{Start: 4, End: 6},
	)
	if err == nil {
		t.Fatal("WriteRange() unexpectedly succeeded")
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output file exists after rejected range: %v", statErr)
	}
}

func TestPDFCPUEngineWriteRangeRejectsEncrypted(t *testing.T) {
	err := NewPDFCPUEngine().WriteRange(
		filepath.Join("testdata", "encrypted.pdf"),
		filepath.Join(t.TempDir(), "out.pdf"),
		domain.PageRange{Start: 1, End: 1},
	)
	if !errors.Is(err, ErrEncrypted) {
		t.Fatalf("WriteRange() error = %v, want ErrEncrypted", err)
	}
}

func TestPDFCPUEngineWriteRangeDoesNotValidateInputFirst(t *testing.T) {
	engine := &pdfcpuEngine{
		inspect: func(string) (Info, error) {
			t.Fatal("WriteRange unexpectedly inspected the input")
			return Info{}, nil
		},
		pages: map[string]int{filepath.Join("testdata", "basic.pdf"): 5},
	}
	output := filepath.Join(t.TempDir(), "pages-2-4.pdf")

	if err := engine.WriteRange(filepath.Join("testdata", "basic.pdf"), output, domain.PageRange{Start: 2, End: 4}); err != nil {
		t.Fatalf("WriteRange() error = %v", err)
	}
}

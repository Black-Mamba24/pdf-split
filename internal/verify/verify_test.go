package verify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

func TestVerifyRejectsPlanGapOrOverlap(t *testing.T) {
	req := validRequest(t)
	req.Plan = domain.SplitPlan{Ranges: []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 3, End: 3},
	}}

	err := Verify(fakeInspector{pages: map[string]int{req.Paths[0]: 1, req.Paths[1]: 1}}, req)
	if err == nil {
		t.Fatal("Verify() error = nil, want invalid plan error")
	}
}

func TestVerifyRejectsUnreadableOutput(t *testing.T) {
	req := validRequest(t)
	inspectErr := errors.New("cannot open pdf")
	inspector := fakeInspector{errByPath: map[string]error{req.Paths[0]: inspectErr}}

	err := Verify(inspector, req)
	if !errors.Is(err, inspectErr) {
		t.Fatalf("Verify() error = %v, want %v", err, inspectErr)
	}
}

func TestVerifyRejectsOutputPageCountMismatch(t *testing.T) {
	req := validRequest(t)
	inspector := fakeInspector{pages: map[string]int{
		req.Paths[0]: 1,
		req.Paths[1]: 1,
	}}

	err := Verify(inspector, req)
	if err == nil {
		t.Fatal("Verify() error = nil, want page count mismatch")
	}
}

func TestVerifyRejectsSummedOutputPagesMismatch(t *testing.T) {
	req := validRequest(t)
	req.TotalPages = 4
	inspector := fakeInspector{pages: map[string]int{
		req.Paths[0]: 1,
		req.Paths[1]: 2,
	}}

	err := Verify(inspector, req)
	if err == nil {
		t.Fatal("Verify() error = nil, want summed pages mismatch")
	}
}

func TestVerifyRejectsNonSinglePageOutputOverMaxBytes(t *testing.T) {
	req := validRequest(t)
	writeBytes(t, req.Paths[1], 101)
	req.MaxBytes = 100
	inspector := validInspector(req)

	err := Verify(inspector, req)
	if err == nil {
		t.Fatal("Verify() error = nil, want oversized multi-page output error")
	}
}

func TestVerifyAcceptsExplicitOversizedSinglePage(t *testing.T) {
	req := validRequest(t)
	writeBytes(t, req.Paths[0], 200)
	req.MaxBytes = 100
	req.OversizedSingles = map[domain.PageRange]struct{}{{Start: 1, End: 1}: {}}
	inspector := validInspector(req)

	if err := Verify(inspector, req); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsUnexpectedOversizedSinglePage(t *testing.T) {
	req := validRequest(t)
	writeBytes(t, req.Paths[0], 200)
	req.MaxBytes = 100
	inspector := validInspector(req)

	err := Verify(inspector, req)
	if err == nil {
		t.Fatal("Verify() error = nil, want unexpected oversized single page error")
	}
}

func TestVerifyRejectsPathCountMismatch(t *testing.T) {
	req := validRequest(t)
	req.Paths = req.Paths[:1]

	err := Verify(validInspector(req), req)
	if err == nil {
		t.Fatal("Verify() error = nil, want path count mismatch")
	}
}

type fakeInspector struct {
	pages     map[string]int
	errByPath map[string]error
}

func (i fakeInspector) Inspect(path string) (pdf.Info, error) {
	if err := i.errByPath[path]; err != nil {
		return pdf.Info{}, err
	}
	return pdf.Info{Pages: i.pages[path]}, nil
}

func validRequest(t *testing.T) Request {
	t.Helper()
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "report-001.pdf"), filepath.Join(dir, "report-002.pdf")}
	writeBytes(t, paths[0], 80)
	writeBytes(t, paths[1], 90)
	return Request{
		TotalPages: 3,
		Plan: domain.SplitPlan{Ranges: []domain.PageRange{
			{Start: 1, End: 1},
			{Start: 2, End: 3},
		}},
		Paths:    paths,
		MaxBytes: 100,
	}
}

func validInspector(req Request) fakeInspector {
	pages := make(map[string]int, len(req.Paths))
	for i, path := range req.Paths {
		if i >= len(req.Plan.Ranges) {
			break
		}
		pages[path] = req.Plan.Ranges[i].Pages()
	}
	return fakeInspector{pages: pages}
}

func writeBytes(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

package pdf

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestCountingWriterAccumulatesBytes(t *testing.T) {
	writer := &countingWriter{}

	for _, data := range [][]byte{[]byte("abc"), nil, []byte("defgh")} {
		got, err := writer.Write(data)
		if err != nil {
			t.Fatalf("Write(%q) error = %v", data, err)
		}
		if got != len(data) {
			t.Fatalf("Write(%q) = %d, want %d", data, got, len(data))
		}
	}
	if writer.bytes != 8 {
		t.Fatalf("accumulated bytes = %d, want 8", writer.bytes)
	}
}

func TestReusableMeasurementContextRejectsNamedDestinations(t *testing.T) {
	ctx := &model.Context{XRefTable: &model.XRefTable{Names: map[string]*model.Node{
		"Dests": {},
	}}}

	if err := validateReusableMeasurementContext(ctx); !errors.Is(err, ErrMeasurementSessionUnsupported) {
		t.Fatalf("validateReusableMeasurementContext() error = %v, want ErrMeasurementSessionUnsupported", err)
	}
}

func TestReusableMeasurementContextAcceptsContextWithoutNamedDestinations(t *testing.T) {
	ctx := &model.Context{XRefTable: &model.XRefTable{Names: map[string]*model.Node{}}}

	if err := validateReusableMeasurementContext(ctx); err != nil {
		t.Fatalf("validateReusableMeasurementContext() error = %v", err)
	}
}

func TestPDFCPUMeasurementSessionReturnsPlanningReferenceSizes(t *testing.T) {
	engine := NewPDFCPUEngine()
	session := openMeasurementSession(t, engine)
	defer session.Close()

	// Separate pdfcpu serializations can differ due to map iteration; final verification is authoritative.
	for _, pageRange := range []domain.PageRange{
		{Start: 1, End: 1},
		{Start: 2, End: 4},
		{Start: 1, End: 5},
	} {
		got, err := session.MeasureRange(pageRange)
		if err != nil {
			t.Fatalf("MeasureRange(%v) error = %v", pageRange, err)
		}
		if got <= 0 {
			t.Fatalf("MeasureRange(%v) = %d, want positive planning reference size", pageRange, got)
		}

		output := filepath.Join(t.TempDir(), "out.pdf")
		if err := engine.WriteRange(filepath.Join("testdata", "basic.pdf"), output, pageRange); err != nil {
			t.Fatalf("WriteRange(%v) error = %v", pageRange, err)
		}
		info, err := os.Stat(output)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() <= 0 {
			t.Fatalf("WriteRange(%v) size = %d, want positive final size", pageRange, info.Size())
		}
	}
}

func TestPDFCPUMeasurementSessionReusesSourceSafely(t *testing.T) {
	session := openMeasurementSession(t, NewPDFCPUEngine())
	defer session.Close()

	ranges := []domain.PageRange{
		{Start: 2, End: 4},
		{Start: 1, End: 3},
		{Start: 4, End: 5},
		{Start: 1, End: 3},
		{Start: 2, End: 4},
	}
	for _, pageRange := range ranges {
		got, err := session.MeasureRange(pageRange)
		if err != nil {
			t.Fatalf("MeasureRange(%v) error = %v", pageRange, err)
		}
		if got <= 0 {
			t.Fatalf("MeasureRange(%v) after reuse = %d, want positive planning reference size", pageRange, got)
		}
	}
}

func TestPDFCPUMeasurementSessionRejectsInvalidRanges(t *testing.T) {
	session := openMeasurementSession(t, NewPDFCPUEngine())
	defer session.Close()

	for _, pageRange := range []domain.PageRange{
		{Start: 0, End: 1},
		{Start: 2, End: 1},
		{Start: 1, End: 6},
		{Start: 6, End: 6},
	} {
		if _, err := session.MeasureRange(pageRange); err == nil {
			t.Fatalf("MeasureRange(%v) unexpectedly succeeded", pageRange)
		}
	}
}

func TestPDFCPUMeasurementSessionRejectsUseAfterClose(t *testing.T) {
	session := openMeasurementSession(t, NewPDFCPUEngine())
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := session.MeasureRange(domain.PageRange{Start: 1, End: 1})
	if !errors.Is(err, ErrMeasurementSessionClosed) {
		t.Fatalf("MeasureRange() error = %v, want ErrMeasurementSessionClosed", err)
	}
}

func TestPDFCPUMeasurementSessionCloseIsIdempotent(t *testing.T) {
	session := openMeasurementSession(t, NewPDFCPUEngine())
	if err := session.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func openMeasurementSession(t *testing.T, engine Engine) MeasurementSession {
	t.Helper()
	opener, ok := engine.(MeasurementSessionOpener)
	if !ok {
		t.Fatal("engine does not implement MeasurementSessionOpener")
	}
	session, err := opener.OpenMeasurementSession(filepath.Join("testdata", "basic.pdf"))
	if err != nil {
		t.Fatalf("OpenMeasurementSession() error = %v", err)
	}
	return session
}

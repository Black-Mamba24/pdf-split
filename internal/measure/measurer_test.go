package measure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

func TestMeasureWritesCandidateAndReturnsActualSize(t *testing.T) {
	tempDir := t.TempDir()
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{
		{Start: 2, End: 4}: 1234,
	}}
	measurer := New(engine, "input.pdf", tempDir, 8)

	got, err := measurer.Measure(context.Background(), domain.PageRange{Start: 2, End: 4})
	if err != nil {
		t.Fatalf("Measure() error = %v", err)
	}
	if got != 1234 {
		t.Fatalf("Measure() = %d, want 1234", got)
	}
	if engine.writes != 1 {
		t.Fatalf("WriteRange() calls = %d, want 1", engine.writes)
	}
	assertInsideTempDir(t, tempDir, engine.paths[0])
	if measurer.Measurements() != 1 {
		t.Fatalf("Measurements() = %d, want 1", measurer.Measurements())
	}
}

func TestMeasureCachesRangeSize(t *testing.T) {
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 99,
	}}
	measurer := New(engine, "input.pdf", t.TempDir(), 8)
	pages := domain.PageRange{Start: 1, End: 1}

	first, err := measurer.Measure(context.Background(), pages)
	if err != nil {
		t.Fatalf("first Measure() error = %v", err)
	}
	second, err := measurer.Measure(context.Background(), pages)
	if err != nil {
		t.Fatalf("second Measure() error = %v", err)
	}
	if first != 99 || second != 99 {
		t.Fatalf("Measure() sizes = %d, %d; want both 99", first, second)
	}
	if engine.writes != 1 {
		t.Fatalf("WriteRange() calls = %d, want 1", engine.writes)
	}
	if measurer.Measurements() != 1 {
		t.Fatalf("Measurements() = %d, want 1", measurer.Measurements())
	}
}

func TestMeasureEvictsLeastRecentlyUsedEntry(t *testing.T) {
	rangeA := domain.PageRange{Start: 1, End: 1}
	rangeB := domain.PageRange{Start: 2, End: 2}
	rangeC := domain.PageRange{Start: 3, End: 3}
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{
		rangeA: 10,
		rangeB: 20,
		rangeC: 30,
	}}
	measurer := New(engine, "input.pdf", t.TempDir(), 2)

	for _, pages := range []domain.PageRange{rangeA, rangeB, rangeA, rangeC, rangeB} {
		if _, err := measurer.Measure(context.Background(), pages); err != nil {
			t.Fatalf("Measure(%+v) error = %v", pages, err)
		}
	}

	if got := engine.calls[rangeA]; got != 1 {
		t.Fatalf("range A writes = %d, want 1", got)
	}
	if got := engine.calls[rangeB]; got != 2 {
		t.Fatalf("range B writes = %d, want 2 after eviction", got)
	}
	if got := engine.calls[rangeC]; got != 1 {
		t.Fatalf("range C writes = %d, want 1", got)
	}
	if measurer.Measurements() != 4 {
		t.Fatalf("Measurements() = %d, want 4", measurer.Measurements())
	}
}

func TestMeasureDeletesCandidateAfterStat(t *testing.T) {
	tempDir := t.TempDir()
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 2}: 128,
	}}
	measurer := New(engine, "input.pdf", tempDir, 8)

	if _, err := measurer.Measure(context.Background(), domain.PageRange{Start: 1, End: 2}); err != nil {
		t.Fatalf("Measure() error = %v", err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir contains %d candidate files after Measure(), want 0", len(entries))
	}
}

func TestMeasureCanceledContextDoesNotWriteCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{
		{Start: 1, End: 1}: 42,
	}}
	measurer := New(engine, "input.pdf", t.TempDir(), 8)

	_, err := measurer.Measure(ctx, domain.PageRange{Start: 1, End: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Measure() error = %v, want context.Canceled", err)
	}
	if engine.writes != 0 {
		t.Fatalf("WriteRange() calls = %d, want 0", engine.writes)
	}
	if measurer.Measurements() != 0 {
		t.Fatalf("Measurements() = %d, want 0", measurer.Measurements())
	}
}

func TestMeasureDeletesCandidateWhenWriteFails(t *testing.T) {
	tempDir := t.TempDir()
	writeErr := errors.New("write failed")
	engine := &fakeEngine{err: writeErr, createBeforeError: true}
	measurer := New(engine, "input.pdf", tempDir, 8)

	_, err := measurer.Measure(context.Background(), domain.PageRange{Start: 1, End: 1})
	if !errors.Is(err, writeErr) {
		t.Fatalf("Measure() error = %v, want %v", err, writeErr)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir contains %d candidate files after failed Measure(), want 0", len(entries))
	}
}

func TestMeasureWithoutCacheStillMeasures(t *testing.T) {
	pages := domain.PageRange{Start: 1, End: 1}
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{pages: 7}}
	measurer := New(engine, "input.pdf", t.TempDir(), 0)

	for i := 0; i < 2; i++ {
		got, err := measurer.Measure(context.Background(), pages)
		if err != nil {
			t.Fatalf("Measure() error = %v", err)
		}
		if got != 7 {
			t.Fatalf("Measure() = %d, want 7", got)
		}
	}
	if engine.writes != 2 {
		t.Fatalf("WriteRange() calls = %d, want 2", engine.writes)
	}
}

func TestMeasureReportsCandidateStartAndCompletion(t *testing.T) {
	pages := domain.PageRange{Start: 2, End: 4}
	engine := &fakeEngine{sizes: map[domain.PageRange]int64{pages: 7}}
	var events []ProgressEvent
	measurer := NewWithProgress(engine, "input.pdf", t.TempDir(), 8, func(event ProgressEvent) {
		events = append(events, event)
	})

	if _, err := measurer.Measure(context.Background(), pages); err != nil {
		t.Fatal(err)
	}
	assertProgressEvents(t, events, pages)
}

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
	assertProgressEvents(t, events, pages)
}

func TestMeasureWithSessionDoesNotCreateCandidateFiles(t *testing.T) {
	tempDir := t.TempDir()
	pages := domain.PageRange{Start: 1, End: 1}
	session := &fakeSession{sizes: map[domain.PageRange]int64{pages: 7}}
	measurer := NewWithSession(session, 8, nil)

	if _, err := measurer.Measure(context.Background(), pages); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir contains %d files after session Measure(), want 0", len(entries))
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
	if measurer.Measurements() != 0 {
		t.Fatalf("Measurements() = %d, want 0", measurer.Measurements())
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
	if measurer.Measurements() != 0 {
		t.Fatalf("Measurements() = %d, want 0", measurer.Measurements())
	}
	if len(events) != 1 || events[0].Done {
		t.Fatalf("events = %#v, want only start event", events)
	}
}

func TestMeasureWithSessionWithoutCacheStillMeasures(t *testing.T) {
	pages := domain.PageRange{Start: 1, End: 1}
	session := &fakeSession{sizes: map[domain.PageRange]int64{pages: 7}}
	measurer := NewWithSession(session, 0, nil)

	for i := 0; i < 2; i++ {
		got, err := measurer.Measure(context.Background(), pages)
		if err != nil {
			t.Fatal(err)
		}
		if got != 7 {
			t.Fatalf("Measure() = %d, want 7", got)
		}
	}
	if session.calls != 2 {
		t.Fatalf("MeasureRange calls = %d, want 2", session.calls)
	}
}

func assertProgressEvents(t *testing.T, events []ProgressEvent, pages domain.PageRange) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("events = %#v, want start and completion", events)
	}
	if events[0].Range != pages || events[0].Completed != 0 || events[0].Done {
		t.Fatalf("start event = %#v", events[0])
	}
	if events[1].Range != pages || events[1].Completed != 1 || !events[1].Done {
		t.Fatalf("completion event = %#v", events[1])
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

type fakeEngine struct {
	mu                sync.Mutex
	sizes             map[domain.PageRange]int64
	calls             map[domain.PageRange]int
	paths             []string
	writes            int
	err               error
	createBeforeError bool
}

func (e *fakeEngine) Inspect(string) (pdf.Info, error) {
	return pdf.Info{}, nil
}

func (e *fakeEngine) WriteRange(_ string, outputPath string, pageRange domain.PageRange) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.calls == nil {
		e.calls = make(map[domain.PageRange]int)
	}
	e.calls[pageRange]++
	e.paths = append(e.paths, outputPath)
	e.writes++

	if e.err != nil {
		if e.createBeforeError {
			if err := os.WriteFile(outputPath, []byte("partial"), 0o600); err != nil {
				return err
			}
		}
		return e.err
	}

	size := e.sizes[pageRange]
	data := make([]byte, size)
	return os.WriteFile(outputPath, data, 0o600)
}

func assertInsideTempDir(t *testing.T, tempDir, path string) {
	t.Helper()
	rel, err := filepath.Rel(tempDir, path)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if rel == "." || rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
		t.Fatalf("candidate path %q is outside temp dir %q", path, tempDir)
	}
}

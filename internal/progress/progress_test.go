package progress

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDisabledProgressWritesNothing(t *testing.T) {
	var stderr bytes.Buffer
	reporter := New(&stderr, false)
	reporter.Planning(3)
	reporter.StartFile(1, 2, "report-001.pdf", 100)
	done := make(chan struct{})
	close(done)
	reporter.WatchFile(context.Background(), filepath.Join(t.TempDir(), "out.pdf"), done)
	reporter.Complete(100)
	reporter.Close()

	if stderr.Len() != 0 {
		t.Fatalf("progress wrote %q, want nothing", stderr.String())
	}
}

func TestPlanningMeasurementsRenderAsActivityNotPercentage(t *testing.T) {
	reporter, stderr := newTestReporter(true)

	reporter.Planning(42)

	got := stderr.String()
	if !strings.Contains(got, "Planning split boundaries") || !strings.Contains(got, "42 measurements completed") {
		t.Fatalf("planning output = %q, want activity with measurement count", got)
	}
	if strings.Contains(got, "%") {
		t.Fatalf("planning output = %q, want no percentage", got)
	}
}

func TestGenerationRemainsBelowHundredUntilComplete(t *testing.T) {
	reporter, stderr := newTestReporter(true)
	tempFile := filepath.Join(t.TempDir(), "report-001.pdf")
	writeBytes(t, tempFile, 100)
	done := make(chan struct{})
	close(done)

	reporter.StartFile(1, 1, "report-001.pdf", 100)
	reporter.WatchFile(context.Background(), tempFile, done)

	got := stderr.String()
	if strings.Contains(got, "100%") {
		t.Fatalf("incomplete progress = %q, must stay below 100%%", got)
	}
	if !strings.Contains(got, " 99%") {
		t.Fatalf("incomplete progress = %q, want 99%% cap", got)
	}
}

func TestCompleteWritesOneRetainedLineWithSize(t *testing.T) {
	reporter, stderr := newTestReporter(true)
	reporter.StartFile(2, 4, "report-002.pdf", 200)
	reporter.Complete(128 * 1024)

	got := stderr.String()
	if !strings.Contains(got, "\n") {
		t.Fatalf("complete output = %q, want retained newline", got)
	}
	if !strings.Contains(got, "[2/4] report-002.pdf") || !strings.Contains(got, "100%") || !strings.Contains(got, "128.0KB") {
		t.Fatalf("complete output = %q, want file label, 100%%, and size", got)
	}
}

func TestProgressWritesOnlyToSuppliedStderrWriter(t *testing.T) {
	var stderr bytes.Buffer
	reporter := New(&stderr, true)
	reporter.Planning(1)
	reporter.Close()

	if stderr.Len() == 0 {
		t.Fatal("supplied stderr writer was not written")
	}
}

func TestNonTerminalProgressWritesRetainedLogLines(t *testing.T) {
	var stderr bytes.Buffer
	reporter := NewWithTerminal(&stderr, true, false)

	reporter.Planning(3)
	reporter.StartFile(1, 1, "report-001.pdf", 100)
	reporter.Complete(100)

	got := stderr.String()
	if strings.Contains(got, "\r") {
		t.Fatalf("non-terminal progress contains carriage return: %q", got)
	}
	if lines := strings.Count(got, "\n"); lines != 3 {
		t.Fatalf("non-terminal progress = %q, want 3 retained log lines", got)
	}
}

func TestNonTerminalProgressSuppressesDuplicateLogLines(t *testing.T) {
	var stderr bytes.Buffer
	reporter := NewWithTerminal(&stderr, true, false)
	done := make(chan struct{})
	close(done)

	reporter.StartFile(1, 1, "report-001.pdf", 100)
	reporter.WatchFile(context.Background(), filepath.Join(t.TempDir(), "missing.pdf"), done)

	if lines := strings.Count(stderr.String(), "\n"); lines != 1 {
		t.Fatalf("non-terminal progress = %q, want one unique log line", stderr.String())
	}
}

func TestWatchFileStopsWhenDoneCloses(t *testing.T) {
	reporter, stderr := newTestReporter(true)
	tempFile := filepath.Join(t.TempDir(), "report-001.pdf")
	writeBytes(t, tempFile, 50)
	done := make(chan struct{})
	close(done)

	reporter.StartFile(1, 1, "report-001.pdf", 100)
	reporter.WatchFile(context.Background(), tempFile, done)

	if !strings.Contains(stderr.String(), "50%") {
		t.Fatalf("watch output = %q, want current file percentage", stderr.String())
	}
}

func newTestReporter(enabled bool) (*reporter, *bytes.Buffer) {
	var stderr bytes.Buffer
	reporter := newReporter(&stderr, enabled, true, func() time.Time { return time.Unix(0, 0) })
	reporter.pollInterval = time.Millisecond
	return reporter, &stderr
}

func writeBytes(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

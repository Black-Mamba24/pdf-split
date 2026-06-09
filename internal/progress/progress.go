package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

type Reporter interface {
	Planning(measurements int)
	PlanningRange(pages domain.PageRange, completed int)
	ScanningPages(measured, total int)
	StartFile(index, total int, name string, expectedBytes int64)
	WatchFile(ctx context.Context, path string, done <-chan struct{})
	Complete(actualBytes int64)
	Close()
}

type reporter struct {
	w            io.Writer
	enabled      bool
	terminal     bool
	now          func() time.Time
	pollInterval time.Duration

	mu            sync.Mutex
	currentIndex  int
	currentTotal  int
	currentName   string
	expectedBytes int64
	lastDynamic   string
	closed        bool
}

func New(w io.Writer, enabled bool) Reporter {
	return newReporter(w, enabled, enabled, time.Now)
}

func NewWithTerminal(w io.Writer, enabled, terminal bool) Reporter {
	return newReporter(w, enabled, terminal, time.Now)
}

func newReporter(w io.Writer, enabled, terminal bool, now func() time.Time) *reporter {
	return &reporter{
		w:            w,
		enabled:      enabled,
		terminal:     terminal,
		now:          now,
		pollInterval: 100 * time.Millisecond,
	}
}

func (r *reporter) Planning(measurements int) {
	if !r.shouldWrite() {
		return
	}
	r.writeDynamic("Planning split boundaries... %d measurements completed", measurements)
}

func (r *reporter) PlanningRange(pages domain.PageRange, completed int) {
	if !r.shouldWrite() {
		return
	}
	r.writeDynamic("Planning split boundaries: measuring pages %d-%d...", pages.Start, pages.End)
}

func (r *reporter) ScanningPages(measured, total int) {
	if !r.shouldWrite() {
		return
	}
	r.writeDynamic("Scanning PDF pages: %d/%d measured", measured, total)
}

func (r *reporter) StartFile(index, total int, name string, expectedBytes int64) {
	if !r.shouldWrite() {
		return
	}
	r.mu.Lock()
	r.currentIndex = index
	r.currentTotal = total
	r.currentName = name
	r.expectedBytes = expectedBytes
	r.mu.Unlock()
	r.renderFileProgress(0, false)
}

func (r *reporter) WatchFile(ctx context.Context, path string, done <-chan struct{}) {
	if !r.shouldWrite() {
		return
	}
	r.renderPath(path, false)
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			r.renderPath(path, false)
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.renderPath(path, false)
		}
	}
}

func (r *reporter) Complete(actualBytes int64) {
	if !r.shouldWrite() {
		return
	}
	r.renderFileProgress(actualBytes, true)
}

func (r *reporter) Close() {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
}

func (r *reporter) renderPath(path string, complete bool) {
	info, err := os.Stat(path)
	if err != nil {
		r.renderFileProgress(0, complete)
		return
	}
	r.renderFileProgress(info.Size(), complete)
}

func (r *reporter) renderFileProgress(actualBytes int64, complete bool) {
	r.mu.Lock()
	index, total, name, expectedBytes := r.currentIndex, r.currentTotal, r.currentName, r.expectedBytes
	r.mu.Unlock()

	percent := progressPercent(actualBytes, expectedBytes, complete)
	bar := progressBar(percent, 20)
	line := fmt.Sprintf("[%d/%d] %s  %s %3d%%  %s", index, total, name, bar, percent, formatBytes(actualBytes))
	if complete {
		r.writeRetained(line)
		return
	}
	r.writeDynamic("%s", line)
}

func (r *reporter) shouldWrite() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled && !r.closed
}

func (r *reporter) writeDynamic(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	r.mu.Lock()
	if line == r.lastDynamic {
		r.mu.Unlock()
		return
	}
	r.lastDynamic = line
	r.mu.Unlock()
	if r.terminal {
		fmt.Fprintf(r.w, "\r%s\033[K", line)
		return
	}
	fmt.Fprintln(r.w, line)
}

func (r *reporter) writeRetained(line string) {
	if r.terminal {
		fmt.Fprintf(r.w, "\r%s\033[K\n", line)
		return
	}
	fmt.Fprintln(r.w, line)
}

func progressPercent(actualBytes, expectedBytes int64, complete bool) int {
	if complete {
		return 100
	}
	if expectedBytes <= 0 || actualBytes <= 0 {
		return 0
	}
	percent := int(actualBytes * 100 / expectedBytes)
	if percent > 99 {
		return 99
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func progressBar(percent, width int) string {
	if width < 1 {
		return ""
	}
	filled := percent * width / 100
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fPB", value/unit)
}

package integration

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

func TestCLIPartsCreatesOrderedOutputs(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "missing", "out")
	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "3", "--output", outputDir)
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	assertOutputPages(t, outputDir, []int{2, 2, 1})
}

func TestCLICombinedNeverProducesFewerThanParts(t *testing.T) {
	outputDir := t.TempDir()
	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "4", "--max-size", "10MB", "--output", outputDir)
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	matches, err := filepath.Glob(filepath.Join(outputDir, "basic-*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) < 4 {
		t.Fatalf("outputs = %d, want at least 4", len(matches))
	}
}

func TestCLIConflictWithoutOverwriteLeavesExistingFile(t *testing.T) {
	outputDir := t.TempDir()
	target := filepath.Join(outputDir, "basic-001.pdf")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "2", "--output", outputDir)
	if result.code != 6 {
		t.Fatalf("exit = %d, stderr = %s; want 6", result.code, result.stderr)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("existing target changed to %q", data)
	}
}

func TestCLIOverwriteReplacesTargets(t *testing.T) {
	outputDir := t.TempDir()
	target := filepath.Join(outputDir, "basic-001.pdf")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "2", "--output", outputDir, "--overwrite")
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	if _, err := pdf.NewPDFCPUEngine().Inspect(target); err != nil {
		t.Fatalf("replacement is not a readable PDF: %v", err)
	}
}

func TestCLIExitCodesAndHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{name: "help", args: []string{"--help"}, code: 0},
		{name: "missing constraint", args: []string{fixture(t, "basic.pdf")}, code: 2},
		{name: "encrypted", args: []string{fixture(t, "encrypted.pdf"), "--parts", "2"}, code: 4},
		{name: "missing input", args: []string{filepath.Join(t.TempDir(), "missing.pdf"), "--parts", "2"}, code: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args...)
			if result.code != tt.code {
				t.Fatalf("exit = %d, stderr = %s; want %d", result.code, result.stderr, tt.code)
			}
		})
	}
}

func TestCLIRedirectedStderrHasNoDynamicProgress(t *testing.T) {
	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "2", "--output", t.TempDir())
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	if strings.Contains(result.stderr, "Planning split boundaries") || strings.Contains(result.stderr, "[1/") {
		t.Fatalf("redirected stderr contains dynamic progress: %q", result.stderr)
	}
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binary(t), args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run CLI: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func binary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pdf-split")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", path, "./cmd/pdf-split")
	cmd.Dir = repoRoot(t)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return path
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "internal", "pdf", "testdata", name)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func assertOutputPages(t *testing.T, outputDir string, want []int) {
	t.Helper()
	engine := pdf.NewPDFCPUEngine()
	for i, pages := range want {
		path := filepath.Join(outputDir, "basic-"+leftPad(i+1, 3)+".pdf")
		info, err := engine.Inspect(path)
		if err != nil {
			t.Fatalf("inspect %q: %v", path, err)
		}
		if info.Pages != pages {
			t.Fatalf("%q pages = %d, want %d", path, info.Pages, pages)
		}
	}
}

func leftPad(value, width int) string {
	text := strconv.Itoa(value)
	return strings.Repeat("0", width-len(text)) + text
}

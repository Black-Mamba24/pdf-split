//go:build visual

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSplitPagesRenderLikeOriginal(t *testing.T) {
	renderer, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("install Poppler pdftoppm to run visual compatibility tests")
	}

	outputDir := t.TempDir()
	result := runCLI(t, fixture(t, "basic.pdf"), "--parts", "5", "--output", outputDir)
	if result.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", result.code, result.stderr)
	}
	originalDir := filepath.Join(t.TempDir(), "original")
	if err := os.MkdirAll(originalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	render(t, renderer, fixture(t, "basic.pdf"), filepath.Join(originalDir, "page"))
	for page := 1; page <= 5; page++ {
		splitDir := filepath.Join(t.TempDir(), "split")
		if err := os.MkdirAll(splitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		render(t, renderer, filepath.Join(outputDir, "basic-"+leftPad(page, 3)+".pdf"), filepath.Join(splitDir, "page"))
		original, err := os.ReadFile(filepath.Join(originalDir, "page-"+strconv.Itoa(page)+".png"))
		if err != nil {
			t.Fatal(err)
		}
		split, err := os.ReadFile(filepath.Join(splitDir, "page-1.png"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, split) {
			t.Fatalf("rendered content differs for original page %d", page)
		}
	}
}

func render(t *testing.T, renderer, input, prefix string) {
	t.Helper()
	cmd := exec.Command(renderer, "-png", "-r", "72", input, prefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render %q: %v\n%s", input, err, output)
	}
}

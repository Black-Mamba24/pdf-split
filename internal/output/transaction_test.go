package output

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestBeginCreatesMissingOutputDirectoryAndParents(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "missing", "nested")

	tx, err := Begin(outputDir, []string{"report-001.pdf"}, false, nil)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer tx.Abort()

	if info, err := os.Stat(outputDir); err != nil || !info.IsDir() {
		t.Fatalf("output dir stat = %v, %v; want directory", info, err)
	}
	if info, err := os.Stat(tx.stageDir); err != nil || !info.IsDir() {
		t.Fatalf("stage dir stat = %v, %v; want directory", info, err)
	}
	if info, err := os.Stat(tx.backupDir); err != nil || !info.IsDir() {
		t.Fatalf("backup dir stat = %v, %v; want directory", info, err)
	}
}

func TestBeginConflictsFailBeforeFinalGenerationWhenOverwriteFalse(t *testing.T) {
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDir, "report-001.pdf"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Begin(outputDir, []string{"report-001.pdf"}, false, nil)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Begin() error = %v, want fs.ErrExist", err)
	}
}

func TestTransactionPublishesStagedFilesOnCommit(t *testing.T) {
	outputDir := t.TempDir()
	tx, err := Begin(outputDir, []string{"report-001.pdf", "report-002.pdf"}, false, nil)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	for i, content := range []string{"one", "two"} {
		if err := os.WriteFile(tx.StagePath(i), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assertFileContent(t, filepath.Join(outputDir, "report-001.pdf"), "one")
	assertFileContent(t, filepath.Join(outputDir, "report-002.pdf"), "two")
	assertNotExists(t, tx.stageDir)
	assertNotExists(t, tx.backupDir)
}

func TestTransactionAbortRemovesStagingAndLeavesExistingTargetsUnchanged(t *testing.T) {
	outputDir := t.TempDir()
	target := filepath.Join(outputDir, "report-001.pdf")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	tx, err := Begin(outputDir, []string{"report-001.pdf"}, true, nil)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := os.WriteFile(tx.StagePath(0), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := tx.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	assertFileContent(t, target, "old")
	assertNotExists(t, tx.stageDir)
	assertNotExists(t, tx.backupDir)
}

func TestTransactionRenameFailureRestoresAllBackups(t *testing.T) {
	outputDir := t.TempDir()
	first := filepath.Join(outputDir, "report-001.pdf")
	second := filepath.Join(outputDir, "report-002.pdf")
	if err := os.WriteFile(first, []byte("old-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("old-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	renameErr := errors.New("rename failed")
	ops := &failingRenameOps{err: renameErr}
	tx, err := Begin(outputDir, []string{"report-001.pdf", "report-002.pdf"}, true, ops)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := os.WriteFile(tx.StagePath(0), []byte("new-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tx.StagePath(1), []byte("new-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	ops.failOldPath = tx.StagePath(1)

	err = tx.Commit()
	if !errors.Is(err, renameErr) {
		t.Fatalf("Commit() error = %v, want %v", err, renameErr)
	}
	assertFileContent(t, first, "old-one")
	assertFileContent(t, second, "old-two")
	assertNotExists(t, tx.stageDir)
	assertNotExists(t, tx.backupDir)
}

func TestStagePathRejectsOutOfRangeIndex(t *testing.T) {
	tx, err := Begin(t.TempDir(), []string{"report-001.pdf"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Abort()
	if got := tx.StagePath(-1); got != "" {
		t.Fatalf("StagePath(-1) = %q, want empty", got)
	}
	if got := tx.StagePath(1); got != "" {
		t.Fatalf("StagePath(1) = %q, want empty", got)
	}
}

type failingRenameOps struct {
	failOldPath string
	err         error
}

func (o *failingRenameOps) Rename(oldPath, newPath string) error {
	if oldPath == o.failOldPath {
		return o.err
	}
	return os.Rename(oldPath, newPath)
}

func (o *failingRenameOps) Remove(path string) error {
	return os.Remove(path)
}

func (o *failingRenameOps) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, want)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want not exist", path, err)
	}
}

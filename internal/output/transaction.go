package output

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type FileOps interface {
	Rename(oldPath, newPath string) error
	Remove(path string) error
	MkdirAll(path string, perm fs.FileMode) error
}

type osFileOps struct{}

func (osFileOps) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osFileOps) Remove(path string) error {
	return os.Remove(path)
}

func (osFileOps) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

type Transaction struct {
	outputDir string
	stageDir  string
	backupDir string
	names     []string
	overwrite bool
	ops       FileOps
	published []string
	backedUp  map[string]string
}

func Begin(outputDir string, names []string, overwrite bool, ops FileOps) (*Transaction, error) {
	if ops == nil {
		ops = osFileOps{}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("output names must not be empty")
	}
	if err := ops.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory %q: %w", outputDir, err)
	}

	for _, name := range names {
		if filepath.Base(name) != name {
			return nil, fmt.Errorf("output name %q must not contain path separators", name)
		}
		if !overwrite {
			if _, err := os.Stat(filepath.Join(outputDir, name)); err == nil {
				return nil, fmt.Errorf("target %q already exists: %w", filepath.Join(outputDir, name), fs.ErrExist)
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("check target %q: %w", filepath.Join(outputDir, name), err)
			}
		}
	}

	stageDir, err := os.MkdirTemp(outputDir, ".pdf-split-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	backupDir, err := os.MkdirTemp(outputDir, ".pdf-split-backup-*")
	if err != nil {
		_ = os.RemoveAll(stageDir)
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	return &Transaction{
		outputDir: outputDir,
		stageDir:  stageDir,
		backupDir: backupDir,
		names:     append([]string(nil), names...),
		overwrite: overwrite,
		ops:       ops,
		backedUp:  make(map[string]string),
	}, nil
}

func (t *Transaction) StagePath(index int) string {
	if index < 0 || index >= len(t.names) {
		return ""
	}
	return filepath.Join(t.stageDir, t.names[index])
}

func (t *Transaction) Commit() error {
	for _, name := range t.names {
		stagePath := filepath.Join(t.stageDir, name)
		targetPath := filepath.Join(t.outputDir, name)
		if t.overwrite {
			if err := t.backupTarget(targetPath, name); err != nil {
				_ = t.rollback()
				return err
			}
		}
		if err := t.ops.Rename(stagePath, targetPath); err != nil {
			_ = t.rollback()
			return fmt.Errorf("publish %q: %w", targetPath, err)
		}
		t.published = append(t.published, targetPath)
	}
	return t.cleanup()
}

func (t *Transaction) Abort() error {
	return t.cleanup()
}

func (t *Transaction) backupTarget(targetPath, name string) error {
	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("check target %q: %w", targetPath, err)
	}

	backupPath := filepath.Join(t.backupDir, name)
	if err := t.ops.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("backup %q: %w", targetPath, err)
	}
	t.backedUp[targetPath] = backupPath
	return nil
}

func (t *Transaction) rollback() error {
	var rollbackErr error
	for i := len(t.published) - 1; i >= 0; i-- {
		if err := t.ops.Remove(t.published[i]); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	for targetPath, backupPath := range t.backedUp {
		if err := t.ops.Rename(backupPath, targetPath); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	rollbackErr = errors.Join(rollbackErr, t.cleanup())
	return rollbackErr
}

func (t *Transaction) cleanup() error {
	var err error
	err = errors.Join(err, os.RemoveAll(t.stageDir))
	err = errors.Join(err, os.RemoveAll(t.backupDir))
	return err
}

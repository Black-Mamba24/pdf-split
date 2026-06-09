package pdf

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

type pdfcpuEngine struct{}

var disableConfigDirOnce sync.Once

func NewPDFCPUEngine() Engine {
	disableConfigDirOnce.Do(api.DisableConfigDir)
	return pdfcpuEngine{}
}

func (pdfcpuEngine) Inspect(path string) (Info, error) {
	if err := api.ValidateFile(path, nil); err != nil {
		return Info{}, classifyInspectError(path, err)
	}

	pages, err := api.PageCountFile(path)
	if err != nil {
		return Info{}, classifyInspectError(path, err)
	}
	return Info{Pages: pages}, nil
}

func (e pdfcpuEngine) WriteRange(inputPath, outputPath string, pageRange domain.PageRange) error {
	info, err := e.Inspect(inputPath)
	if err != nil {
		return err
	}
	if pageRange.Start < 1 || pageRange.End < pageRange.Start || pageRange.End > info.Pages {
		return fmt.Errorf("page range %d-%d is outside page bounds 1-%d", pageRange.Start, pageRange.End, info.Pages)
	}

	selectedPages := []string{fmt.Sprintf("%d-%d", pageRange.Start, pageRange.End)}
	if err := api.TrimFile(inputPath, outputPath, selectedPages, nil); err != nil {
		if isEncryptionError(err) {
			return fmt.Errorf("%w: write page range from %q: %w", ErrEncrypted, inputPath, err)
		}
		return fmt.Errorf("write page range %d-%d from %q to %q: %w", pageRange.Start, pageRange.End, inputPath, outputPath, err)
	}
	return nil
}

func classifyInspectError(path string, err error) error {
	if isEncryptionError(err) {
		return fmt.Errorf("%w: inspect %q: %w", ErrEncrypted, path, err)
	}

	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return fmt.Errorf("inspect %q: %w", path, err)
	}
	return fmt.Errorf("%w: inspect %q: %w", ErrInvalid, path, err)
}

func isEncryptionError(err error) bool {
	if errors.Is(err, pdfcpu.ErrWrongPassword) || errors.Is(err, pdfcpu.ErrUnknownEncryption) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "password") || strings.Contains(message, "encrypt")
}

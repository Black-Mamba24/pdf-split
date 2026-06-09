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
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type pdfcpuEngine struct {
	inspect func(path string) (Info, error)
	mu      sync.RWMutex
	pages   map[string]int
}

var disableConfigDirOnce sync.Once

func NewPDFCPUEngine() Engine {
	disableConfigDirOnce.Do(api.DisableConfigDir)
	return &pdfcpuEngine{pages: make(map[string]int)}
}

func (e *pdfcpuEngine) Inspect(path string) (Info, error) {
	if e.inspect != nil {
		return e.inspect(path)
	}
	if err := api.ValidateFile(path, fastConfiguration()); err != nil {
		return Info{}, classifyInspectError(path, err)
	}

	pages, err := api.PageCountFile(path)
	if err != nil {
		return Info{}, classifyInspectError(path, err)
	}
	e.storePages(path, pages)
	return Info{Pages: pages}, nil
}

func (e *pdfcpuEngine) WriteRange(inputPath, outputPath string, pageRange domain.PageRange) error {
	if pageRange.Start < 1 || pageRange.End < pageRange.Start {
		return fmt.Errorf("invalid page range %d-%d", pageRange.Start, pageRange.End)
	}
	pages, ok := e.cachedPages(inputPath)
	if !ok {
		var err error
		pages, err = api.PageCountFile(inputPath)
		if err != nil {
			return classifyInspectError(inputPath, err)
		}
		e.storePages(inputPath, pages)
	}
	if pageRange.End > pages {
		return fmt.Errorf("page range %d-%d is outside page bounds 1-%d", pageRange.Start, pageRange.End, pages)
	}

	selectedPages := []string{fmt.Sprintf("%d-%d", pageRange.Start, pageRange.End)}
	if err := api.TrimFile(inputPath, outputPath, selectedPages, fastConfiguration()); err != nil {
		if isEncryptionError(err) {
			return fmt.Errorf("%w: write page range from %q: %w", ErrEncrypted, inputPath, err)
		}
		return fmt.Errorf("write page range %d-%d from %q to %q: %w", pageRange.Start, pageRange.End, inputPath, outputPath, err)
	}
	return nil
}

func fastConfiguration() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.Optimize = false
	conf.OptimizeBeforeWriting = false
	conf.PostProcessValidate = false
	return conf
}

func (e *pdfcpuEngine) cachedPages(path string) (int, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pages, ok := e.pages[path]
	return pages, ok
}

func (e *pdfcpuEngine) storePages(path string, pages int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pages == nil {
		e.pages = make(map[string]int)
	}
	e.pages[path] = pages
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

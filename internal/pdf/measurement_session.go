package pdf

import (
	"fmt"
	"os"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type pdfcpuMeasurementSession struct {
	source    *os.File
	context   *model.Context
	pageCount int
	closed    bool
}

func (e *pdfcpuEngine) OpenMeasurementSession(path string) (MeasurementSession, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open measurement session for %q: %w", path, err)
	}

	conf := fastConfiguration()
	conf.Cmd = model.TRIM
	context, err := api.ReadValidateAndOptimize(source, conf)
	if err != nil {
		_ = source.Close()
		return nil, classifyInspectError(path, err)
	}
	if err := validateReusableMeasurementContext(context); err != nil {
		_ = source.Close()
		return nil, err
	}
	e.storePages(path, context.PageCount)

	return &pdfcpuMeasurementSession{
		source:    source,
		context:   context,
		pageCount: context.PageCount,
	}, nil
}

func validateReusableMeasurementContext(context *model.Context) error {
	if context != nil && context.XRefTable != nil {
		if _, ok := context.Names["Dests"]; ok {
			return fmt.Errorf("%w: named destinations mutate the source context during page extraction", ErrMeasurementSessionUnsupported)
		}
	}
	return nil
}

func (s *pdfcpuMeasurementSession) MeasureRange(pageRange domain.PageRange) (int64, error) {
	if s.closed {
		return 0, ErrMeasurementSessionClosed
	}
	if pageRange.Start < 1 || pageRange.End < pageRange.Start {
		return 0, fmt.Errorf("invalid page range %d-%d", pageRange.Start, pageRange.End)
	}
	if pageRange.End > s.pageCount {
		return 0, fmt.Errorf("page range %d-%d is outside page bounds 1-%d", pageRange.Start, pageRange.End, s.pageCount)
	}

	pageNumbers := make([]int, 0, pageRange.Pages())
	for page := pageRange.Start; page <= pageRange.End; page++ {
		pageNumbers = append(pageNumbers, page)
	}
	context, err := pdfcpu.ExtractPages(s.context, pageNumbers, false)
	if err != nil {
		return 0, fmt.Errorf("extract measurement range %d-%d: %w", pageRange.Start, pageRange.End, err)
	}

	counter := &countingWriter{}
	if err := api.WriteContext(context, counter); err != nil {
		return 0, fmt.Errorf("write measurement range %d-%d: %w", pageRange.Start, pageRange.End, err)
	}
	return counter.bytes, nil
}

func (s *pdfcpuMeasurementSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.context = nil
	source := s.source
	s.source = nil
	if source == nil {
		return nil
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("close measurement session: %w", err)
	}
	return nil
}

type countingWriter struct {
	bytes int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.bytes += int64(len(p))
	return len(p), nil
}

package pdf

import (
	"errors"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

var (
	ErrEncrypted                     = errors.New("encrypted PDF is not supported")
	ErrInvalid                       = errors.New("invalid PDF")
	ErrMeasurementSessionUnsupported = errors.New("measurement session is unsupported")
	ErrMeasurementSessionClosed      = errors.New("measurement session is closed")
)

type Info struct {
	Pages     int
	Encrypted bool
}

type Engine interface {
	Inspect(path string) (Info, error)
	WriteRange(inputPath, outputPath string, pageRange domain.PageRange) error
}

type MeasurementSession interface {
	MeasureRange(pageRange domain.PageRange) (int64, error)
	Close() error
}

type MeasurementSessionOpener interface {
	OpenMeasurementSession(inputPath string) (MeasurementSession, error)
}

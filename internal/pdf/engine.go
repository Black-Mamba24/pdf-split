package pdf

import (
	"errors"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
)

var (
	ErrEncrypted = errors.New("encrypted PDF is not supported")
	ErrInvalid   = errors.New("invalid PDF")
)

type Info struct {
	Pages     int
	Encrypted bool
}

type Engine interface {
	Inspect(path string) (Info, error)
	WriteRange(inputPath, outputPath string, pageRange domain.PageRange) error
}

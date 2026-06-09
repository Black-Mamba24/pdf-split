package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const pageCount = 5

func main() {
	api.DisableConfigDir()

	dir, err := filepath.Abs(filepath.Join("internal", "pdf", "testdata"))
	if err != nil {
		panic(err)
	}
	tmpDir, err := os.MkdirTemp("", "pdf-split-fixtures-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	imageFiles := make([]string, 0, pageCount)
	for page := 1; page <= pageCount; page++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("page-%d.png", page))
		if err := writePageImage(path, page); err != nil {
			panic(err)
		}
		imageFiles = append(imageFiles, path)
	}

	basic := filepath.Join(dir, "basic.pdf")
	encrypted := filepath.Join(dir, "encrypted.pdf")
	if err := os.Remove(basic); err != nil && !os.IsNotExist(err) {
		panic(err)
	}
	if err := api.ImportImagesFile(imageFiles, basic, nil, nil); err != nil {
		panic(err)
	}
	if err := api.EncryptFile(basic, encrypted, model.NewAESConfiguration("test", "owner", 256)); err != nil {
		panic(err)
	}
}

func writePageImage(path string, page int) error {
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	background := color.RGBA{R: uint8(30 * page), G: uint8(255 - 25*page), B: uint8(35 * page), A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

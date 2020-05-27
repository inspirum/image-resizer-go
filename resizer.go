package main

import (
	"errors"
	"fmt"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Template struct {
	width     float64
	height    float64
	ratio     float64
	crop      bool
	upscale   bool
	inputExt  string
	outputExt string
}

func NewTemplate(template string, inputExt string, outputExt string) *Template {
	width := 0.0
	height := 0.0
	ratio := 0.0
	crop := false
	upscale := false

	parts := strings.Split(template, "-")
	for _, part := range parts {
		if part[0] == 'w' {
			width, _ = strconv.ParseFloat(part[1:], 64)
		} else if part[0] == 'h' {
			height, _ = strconv.ParseFloat(part[1:], 64)
		} else if strings.Contains(part, "x") {
			sides := strings.SplitN(part, "x", 2)
			ratioWidth, _ := strconv.ParseFloat(sides[0], 64)
			ratioHeight, _ := strconv.ParseFloat(sides[1], 64)
			if ratioHeight > 0 {
				ratio = ratioWidth / ratioHeight
			}
		} else if part == "crop" {
			crop = true
		} else if part == "upscale" {
			upscale = true
		}
	}

	return &Template{width, height, ratio, crop, upscale, inputExt, outputExt}
}

func (t *Template) getDimensions(originalWidth float64, originalHeight float64) (int, int) {
	outputWidth := originalWidth
	outputHeight := originalHeight
	originalRatio := originalWidth / originalHeight

	if t.width > 0 && t.height > 0 {
		outputWidth = t.width
		outputHeight = t.height
	} else if t.width > 0 && t.ratio > 0 {
		outputWidth = t.width
		outputHeight = t.width / t.ratio
	} else if t.height > 0 && t.ratio > 0 {
		outputWidth = t.height * t.ratio
		outputHeight = t.height
	} else if t.ratio > 0 {
		if originalRatio < t.ratio {
			outputWidth = originalHeight * t.ratio
			outputHeight = originalHeight
		} else {
			outputWidth = originalWidth
			outputHeight = originalWidth / t.ratio
		}
	} else if t.width > 0 {
		outputWidth = t.width
		outputHeight = t.width / originalRatio
	} else if t.height > 0 {
		outputWidth = t.height * originalRatio
		outputHeight = t.height
	}

	if t.crop && !t.upscale {
		upscaleWidth := originalWidth / outputWidth
		upscaleHeight := originalHeight / outputHeight
		outputRatio := outputWidth / outputHeight

		if upscaleWidth < 1 && upscaleWidth < upscaleHeight {
			outputWidth = originalWidth
			outputHeight = originalWidth / outputRatio
		} else if upscaleHeight < 1 && upscaleHeight < upscaleWidth {
			outputWidth = originalHeight * outputRatio
			outputHeight = originalHeight
		}
	}

	return int(math.Round(outputWidth)), int(math.Round(outputHeight))
}

func ResizeImage(f *os.File, outputFile string, template *Template) (*os.File, error) {
	defer f.Close()
	defer os.Remove(f.Name())

	c, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}

	outputWidth, outputHeight := template.getDimensions(float64(c.Width), float64(c.Height))

	resizeGeometryArg := "%dx%d>"
	backgroundArg := "none"
	gravityArg := "center"
	extentArg := "%dx%d"

	if template.crop {
		resizeGeometryArg = "%dx%d^"
	} else if template.upscale {
		resizeGeometryArg = "%dx%d"
	}

	if template.outputExt == ".jpg" || template.outputExt == ".jpeg" {
		backgroundArg = "white"
	}

	args := []string{
		f.Name(),
		"-resize", fmt.Sprintf(resizeGeometryArg, outputWidth, outputHeight),
		"-background", backgroundArg,
		"-gravity", gravityArg,
		"-extent", fmt.Sprintf(extentArg, outputWidth, outputHeight),
		outputFile,
	}

	_ = os.MkdirAll(filepath.Dir(outputFile), 0700)

	cmd := exec.Command("convert", args...)

	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	return os.Open(outputFile)
}

func validateFilename(ext string) error {
	supportedFormats := [...]string{
		".jpg",
		".jpeg",
		".png",
		".gif",
		".webp",
		".svg",
		".bmp",
		".tiff",
		".pdf",
	}

	for _, f := range supportedFormats {
		if f == ext {
			return nil
		}
	}

	return errors.New("not supported file format")
}

func isOriginalTemplate(template string, ext string) bool {
	return template == "original" || ext == ".svg" || ext == ".pdf"
}

func validateTemplate(template string) error {
	if !strings.HasPrefix(template, "custom") {
		return errors.New("not supported template")
	}

	return nil
}

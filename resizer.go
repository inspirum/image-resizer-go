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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Template struct {
	width   float64
	height  float64
	ratio   float64
	crop    bool
	upscale bool
}

var convertCmd = GetEnv("CMD_CONVERT", "convert")

var optimizerCmd = map[string]string{
	"png":  GetEnv("CMD_OPTIMIZER_PNG", "pngquant --force --ext .png --skip-if-larger --quality 0-80 --speed 4 --strip -- %file"),
	"jpg":  GetEnv("CMD_OPTIMIZER_JPG", "jpegoptim --force --strip-all --max 70 --quit --all-progressive %file"),
	"gif":  GetEnv("CMD_OPTIMIZER_GIF", "gifsicle --batch --optimize=3 %file"),
	"svg":  GetEnv("CMD_OPTIMIZER_SVG", "svgcleaner %file %file"),
	"webp": GetEnv("CMD_OPTIMIZER_WEBP", "cwebp -m 6 -pass 10 -mt -q 90 %file"),
}

var optimizerCmdExtMapper = map[string]string{
	".png":  "png",
	".jpg":  "jpg",
	".jpeg": "jpg",
	".gif":  "gif",
	".webp": "webp",
}

var supportedFormats = [...]string{
	".webp",
	".png",
	".svg",
	".jpg",
	".gif",
	".jpeg",
	".bmp",
	".tiff",
	".pdf",
}

func NewTemplate(template string) *Template {
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

	return &Template{width, height, ratio, crop, upscale}
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

func GetImageDimensions(p string) (w int, h int, err error) {
	f, err := os.Open(p)
	if err != nil {
		return
	}

	c, _, err := image.DecodeConfig(f)
	if err != nil {
		return
	}

	return c.Width, c.Height, nil
}

func ConvertFile(inputFilePath string, inputWidth int, inputHeight int, outputFilePath string, template *Template) (err error) {
	outputWidth, outputHeight := template.getDimensions(float64(inputWidth), float64(inputHeight))
	outputExt := strings.ToLower(filepath.Ext(outputFilePath))
	var resizeGeometryArg, backgroundArg string

	if template.crop {
		resizeGeometryArg = "%dx%d^"
	} else if template.upscale {
		resizeGeometryArg = "%dx%d"
	} else {
		resizeGeometryArg = "%dx%d>"
	}

	if outputExt == ".jpg" || outputExt == ".jpeg" {
		backgroundArg = "white"
	} else {
		backgroundArg = "none"
	}

	args := []string{
		inputFilePath,
		"-resize", fmt.Sprintf(resizeGeometryArg, outputWidth, outputHeight),
		"-background", backgroundArg,
		"-gravity", "center",
		"-extent", fmt.Sprintf("%dx%d", outputWidth, outputHeight),
		outputFilePath,
	}

	_ = os.MkdirAll(filepath.Dir(outputFilePath), 0777)

	logger("RESIZE %s %v\n", convertCmd, args)
	err = exec.Command(convertCmd, args...).Run()

	return
}

func OptimizeFile(filePath string) (err error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	optimizer, found := optimizerCmd[optimizerCmdExtMapper[ext]]
	if !found || optimizer == "" {
		return nil
	}

	args := strings.Split(strings.ReplaceAll(optimizer, "%file", filePath), " ")

	logger("OPTIMIZE %v\n", args)
	err = exec.Command(args[0], args[1:]...).Run()

	return
}

func validateFilename(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))

	for _, f := range supportedFormats {
		if f == ext {
			return nil
		}
	}

	return HttpError{errors.New("not supported file format"), http.StatusBadRequest}
}

func validateTemplate(template string) error {
	if template != "original" && !strings.HasPrefix(template, "custom") {
		return HttpError{errors.New("not supported template"), http.StatusBadRequest}
	}

	return nil
}

func shouldNotResize(template string, filePath string) bool {
	if template == "original" {
		return true
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	return ext == ".svg" || ext == ".pdf"
}

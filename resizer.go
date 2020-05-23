package main

import (
	"errors"
	"fmt"
	"github.com/discordapp/lilliput"
	"strconv"
	"strings"
)

var EncodeOptions = map[string]map[int]int{
	".jpeg": map[int]int{lilliput.JpegQuality: 85},
	".png":  map[int]int{lilliput.PngCompression: 7},
	".webp": map[int]int{lilliput.WebpQuality: 85},
}

type Template struct {
	width   float64
	height  float64
	ratio   float64
	crop    bool
	upscale bool
}

func isOriginalTemplate(template string) bool {
	return template == "original"
}

func validateTemplate(template string) error {
	if !strings.HasPrefix(template, "custom") {
		return errors.New("not supported template")
	}

	return nil
}

func NewTemplate(template string) (*Template, error) {
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
			tempWidth, _ := strconv.ParseFloat(sides[0], 64)
			tempHeight, _ := strconv.ParseFloat(sides[1], 64)
			if tempHeight > 0 {
				ratio = tempWidth / tempHeight
			}
		} else if part == "crop" {
			crop = true
		} else if part == "upscale" {
			upscale = true
		}
	}

	return &Template{
		width,
		height,
		ratio,
		crop,
		upscale,
	}, nil
}

func (t *Template) getFinal(originalWidth float64, originalHeight float64) (int, int) {

	fmt.Printf("Template config w: %f, h: %f, ratio: %f, crop: %v, upscale: %v\n", t.width, t.height, t.ratio, t.crop, t.upscale)

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

	return int(outputWidth), int(outputHeight)
}

func resizeImage(content []byte, template *Template) ([]byte, error) {

	decoder, err := lilliput.NewDecoder(content)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error decoding image: %s", err))
	}
	defer decoder.Close()

	header, err := decoder.Header()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error reading image header, %s", err))
	}

	// get ready to resize image, using 8192x8192 maximum resize buffer size
	ops := lilliput.NewImageOps(8192)
	defer ops.Close()

	// create a buffer to store the output image, 50MB in this case
	resizedImageContent := make([]byte, 50*1024*1024)

	// use user supplied filename to guess output type if provided
	// otherwise don't transcode (use existing type)
	outputType := "." + strings.ToLower(decoder.Description())

	finalWidth, finalHeight := template.getFinal(float64(header.Width()), float64(header.Height()))

	fmt.Printf("Resizing image to %dx%d\n", finalWidth, finalHeight)

	resizeMethod := lilliput.ImageOpsFit
	if template.crop {
		resizeMethod = lilliput.ImageOpsResize
	}

	if finalWidth == header.Width() && finalHeight == header.Height() {
		resizeMethod = lilliput.ImageOpsNoResize
	}

	opts := &lilliput.ImageOptions{
		FileType:             outputType,
		Width:                finalWidth,
		Height:               finalHeight,
		ResizeMethod:         resizeMethod,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[outputType],
	}

	// resize and transcode image
	resizedImageContent, err = ops.Transform(decoder, opts, resizedImageContent)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error transforming image, %s\n", err))
	}

	return resizedImageContent, nil
}

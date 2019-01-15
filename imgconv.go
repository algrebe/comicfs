package main

import (
	"bytes"
	"fmt"
	"golang.org/x/image/webp"
	"image"
	"image/png"
	"io"
	"path/filepath"

	log "github.com/inconshreveable/log15"
)

type ImageConverterDetector interface {
	Detect(string) (string, ImageConverter)
}

type ImageConverter interface {
	Convert([]byte) ([]byte, error)
}

type SimpleImageConverter struct {
	extToDecoder map[string]func(io.Reader) (image.Image, error)
	extToEncoder map[string]func(io.Writer, image.Image) error
}

func (ic *SimpleImageConverter) Init() {
	ic.extToDecoder = map[string]func(io.Reader) (image.Image, error){
		".webp": webp.Decode,
		".png":  png.Decode,
	}

	ic.extToEncoder = map[string]func(io.Writer, image.Image) error{
		".png": png.Encode,
	}
}

func (ic *SimpleImageConverter) Convert(img []byte, srcExt, dstExt string) ([]byte, error) {
	if srcExt == dstExt {
		return img, nil
	}

	decodeFn, ok := ic.extToDecoder[srcExt]
	if !ok {
		return nil, fmt.Errorf("No such decoder: %s", srcExt)
	}

	encodeFn, ok := ic.extToEncoder[dstExt]
	if !ok {
		return nil, fmt.Errorf("No such encoder: %s", dstExt)
	}

	decodedImg, err := decodeFn(bytes.NewReader(img))
	if err != nil {
		log.Error("error here")
		return nil, err
	}

	var b bytes.Buffer
	if err := encodeFn(&b, decodedImg); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (ic *SimpleImageConverter) Detect(path string) (string, ImageConverter) {
	// *.ext1.ext2 --> *.extToDecode.extToEncode or *.srcExt.dstExt
	ext2 := filepath.Ext(path)
	if ext2 == "" {
		return "", nil
	}

	newPath := path[:len(path)-len(ext2)]
	ext1 := filepath.Ext(newPath)

	if ext1 == "" {
		return "", nil
	}

	if _, ok := ic.extToDecoder[ext1]; !ok {
		return "", nil
	}

	if _, ok := ic.extToEncoder[ext2]; !ok {
		return "", nil
	}

	return newPath, &SimpleImageConverterCtx{ic: ic, srcExt: ext1, dstExt: ext2}
}

type SimpleImageConverterCtx struct {
	ic     *SimpleImageConverter
	srcExt string
	dstExt string
}

func (sicc *SimpleImageConverterCtx) String() string {
	return fmt.Sprintf("SimpleImageConverterCtx<%s, %s>", sicc.srcExt, sicc.dstExt)
}

func (sicc *SimpleImageConverterCtx) Convert(img []byte) ([]byte, error) {
	return sicc.ic.Convert(img, sicc.srcExt, sicc.dstExt)
}

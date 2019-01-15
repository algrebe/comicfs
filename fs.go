package main

import (
	"path/filepath"

	"bazil.org/fuse/fs"
)

type ComicHandler interface {
	fs.Node
	fs.HandleReadDirAller
	fs.NodeRequestLookuper
}

type ComicHandlerCreator func(path string, ig InodeGenerator) (ComicHandler, error)

type FS struct {
	ComicDir                  string
	comicTypeToHandlerCreator map[string]ComicHandlerCreator
	ig                        InodeGenerator
	ic                        ImageConverterDetector
}

func (f *FS) Init() {
	f.comicTypeToHandlerCreator = make(map[string]ComicHandlerCreator)
	sig := &SimpleInodeGenerator{}
	sig.Init()

	sic := &SimpleImageConverter{}
	sic.Init()

	f.ig = sig
	f.ic = sic
}

func (f *FS) Root() (fs.Node, error) {
	return &Dir{path: f.ComicDir, fs: f}, nil
}

func (f *FS) RegisterComicType(suffix string, chc ComicHandlerCreator) {
	f.comicTypeToHandlerCreator[suffix] = chc
}

func (f *FS) GetComicHandlerCreator(path string) ComicHandlerCreator {
	ext := filepath.Ext(path)
	if chc, ok := f.comicTypeToHandlerCreator[ext]; ok {
		return chc
	}
	return nil
}

package main

import (
	"path/filepath"
	"sync"

	"bazil.org/fuse/fs"
)

type FS struct {
	ComicDir      string
	ComicTypes    map[string]struct{}
	inodeMap      map[string]uint64
	inodeMapMutex *sync.Mutex
}

func (f *FS) Init() error {
	f.inodeMap = make(map[string]uint64)
	f.inodeMapMutex = &sync.Mutex{}
	return nil
}

func (f *FS) Root() (fs.Node, error) {
	return &Dir{path: f.ComicDir, fs: f}, nil
}

func (f *FS) GetInode(path string) uint64 {
	f.inodeMapMutex.Lock()
	defer f.inodeMapMutex.Unlock()

	if i, ok := f.inodeMap[path]; ok {
		return i
	}

	f.inodeMap[path] = uint64(len(f.inodeMap))
	return f.inodeMap[path]
}

func (f *FS) IsComic(path string) bool {
	ext := filepath.Ext(path)
	if _, ok := f.ComicTypes[ext]; ok {
		return true
	}
	return false
}

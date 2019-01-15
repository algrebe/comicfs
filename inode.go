package main

import (
	"sync"
)

type InodeGenerator interface {
	GenerateInode(string) uint64
}

type SimpleInodeGenerator struct {
	inodeMap      map[string]uint64
	inodeMapMutex *sync.Mutex
}

func (ig *SimpleInodeGenerator) Init() {
	ig.inodeMap = make(map[string]uint64)
	ig.inodeMapMutex = &sync.Mutex{}
}

func (ig *SimpleInodeGenerator) GenerateInode(path string) uint64 {
	ig.inodeMapMutex.Lock()
	defer ig.inodeMapMutex.Unlock()

	if i, ok := ig.inodeMap[path]; ok {
		return i
	}

	ig.inodeMap[path] = uint64(len(ig.inodeMap))
	return ig.inodeMap[path]
}

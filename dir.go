package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	log "github.com/inconshreveable/log15"
	"golang.org/x/net/context"
)

type Dir struct {
	path string
	fs   *FS
}

func (d *Dir) String() string {
	return fmt.Sprintf("Dir<%s>", d.path)
}

func (d *Dir) Attr(ctx context.Context, attr *fuse.Attr) error {
	fi, err := os.Stat(d.path)
	if err != nil {
		log.Error("Failed to stat dir", "dir", d)
		return err
	}

	attr.Valid = 1 * time.Hour
	attr.Inode = d.fs.ig.GenerateInode(d.path)
	attr.Size = uint64(fi.Size())
	attr.Mode = fi.Mode()
	attr.Mtime = fi.ModTime()
	return nil
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	files, err := ioutil.ReadDir(d.path)
	if err != nil {
		return nil, err
	}

	dirents := make([]fuse.Dirent, len(files))
	for i, f := range files {
		ft := fuse.DT_File
		if f.IsDir() {
			ft = fuse.DT_Dir
		}

		name := f.Name()
		if ft == fuse.DT_File {
			// If we have a comic handler for this name, then we treat it as a directory
			if handler := d.fs.GetComicHandlerCreator(name); handler != nil {
				ft = fuse.DT_Dir
			}
		}

		dirent := fuse.Dirent{
			Type: ft,
			Name: name,
		}

		dirents[i] = dirent
	}

	return dirents, nil
}

func (d *Dir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	path := req.Name
	path = filepath.Join(d.path, path)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fuse.ENOENT
	}

	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return &Dir{path: path, fs: d.fs}, nil
	}

	if chc := d.fs.GetComicHandlerCreator(path); chc != nil {
		handler, err := chc(path, d.fs.ig)
		if err != nil {
			log.Error("Failed to create handler for comic", "dir", d, "path", path, "error", err)
		}

		return handler, err
	}

	return &File{path: path, fs: d.fs}, nil

}

package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	log "github.com/inconshreveable/log15"
	"golang.org/x/net/context"
)

type File struct {
	path string
	fs   *FS
}

func (f *File) String() string {
	return fmt.Sprintf("File<%s>", f.path)
}

func (f *File) Attr(ctx context.Context, attr *fuse.Attr) error {
	fi, err := os.Stat(f.path)
	if err != nil {
		log.Error("Failed to stat file", "file", f, "error", err)
		return err
	}

	attr.Valid = 1 * time.Hour
	attr.Inode = f.fs.GetInode(f.path)
	attr.Size = uint64(fi.Size())
	attr.Mode = fi.Mode()
	attr.Mtime = fi.ModTime()
	return nil
}

func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	fp, err := os.Open(f.path)
	if err != nil {
		log.Error("Failed to open file", "file", f, "error", err)
		return nil, err
	}

	// TODO individual entries inside a zip file are not seekable
	// resp.Flags |= fuse.OpenNonSeekable

	return &FileHandle{file: f, fp: fp}, nil
}

type FileHandle struct {
	file *File
	fp   *os.File
}

func (fh *FileHandle) String() string {
	return fmt.Sprintf("FileHandle<%s>", fh.file.path)
}

func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := fh.fp.Close(); err != nil {
		return err
	}
	return nil
}

func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Debug("filehandle.Read", "filehandle", fh, "offset", req.Offset, "size", req.Size)
	buf := make([]byte, req.Size)
	n, err := fh.fp.ReadAt(buf, req.Offset)
	resp.Data = buf[:n]
	if err == io.EOF {
		err = nil
	}

	if err != nil {
		log.Error("Failed to read file", "filehandle", fh, "error", err)
	}

	return err
}

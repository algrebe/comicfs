package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	log "github.com/inconshreveable/log15"
	"golang.org/x/net/context"
)

type ZipFile struct {
	path          string
	archiveOpened bool
	archive       *zip.ReadCloser
	fileInfo      os.FileInfo
}

func (zf *ZipFile) String() string {
	return fmt.Sprintf("ZipFile<%s>", zf.path)
}

func (zf *ZipFile) Init() error {
	fi, err := os.Stat(zf.path)
	if err != nil {
		return err
	}
	zf.fileInfo = fi

	return nil
}

func (zf *ZipFile) EnsureArchiveOpen() error {
	if zf.archiveOpened {
		return nil
	}

	archive, err := zip.OpenReader(zf.path)
	if err != nil {
		return err
	}

	zf.archive = archive
	zf.archiveOpened = true
	// TODO find a safer way to close archives?
	runtime.SetFinalizer(zf, ZipFileFinalizer)
	return nil
}

func ZipFileFinalizer(zf *ZipFile) {
	log.Debug("Running finalizer for zipfile", "ZipFile", zf)

	if err := zf.archive.Close(); err != nil {
		log.Error("Failed to close zipfile archive", "ZipFile", zf, "error", err)
	}

	zf.archiveOpened = false
	zf.archive = nil
}

type ZipDir struct {
	*ZipFile
	path string
	fs   *FS
}

func MakeZipDir(path string, fs *FS) (*ZipDir, error) {
	zf := &ZipFile{path: path}
	if err := zf.Init(); err != nil {
		log.Error("Failed to initialize zipfile", "path", path, "error", err)
		return nil, err
	}

	return &ZipDir{ZipFile: zf, path: "", fs: fs}, nil
}

func (z *ZipDir) String() string {
	return fmt.Sprintf("ZipDir<%s/%s>", z.ZipFile.path, z.path)
}

func (z *ZipDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Valid = 1 * time.Hour
	attr.Inode = z.fs.GetInode(z.ZipFile.path + z.path)
	attr.Size = uint64(z.fileInfo.Size())
	attr.Mode = os.ModeDir | 0644
	attr.Mtime = z.fileInfo.ModTime()
	return nil
}

func (z *ZipDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	if err := z.EnsureArchiveOpen(); err != nil {
		return nil, err
	}

	dirents := make([]fuse.Dirent, 0)
	for _, f := range z.archive.File {

		if !strings.HasPrefix(f.Name, z.path) {
			continue
		}

		name := f.Name[len(z.path):]
		if name == "" {
			continue
		}

		if strings.ContainsRune(name[:len(name)-1], '/') {
			continue
		}

		dirent := fuse.Dirent{
			Type: fuse.DT_File,
			Name: name,
		}

		if name[len(name)-1] == '/' {
			dirent.Type = fuse.DT_Dir
			dirent.Name = name[:len(name)-1]
		}

		dirents = append(dirents, dirent)
	}

	return dirents, nil
}

func (z *ZipDir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	if err := z.EnsureArchiveOpen(); err != nil {
		return nil, err
	}

	path := filepath.Join(z.path, req.Name)

	for _, f := range z.archive.File {
		switch {
		case f.Name == path:
			fpath := filepath.Join(z.path, f.Name)
			zm := &ZipMember{
				ZipFile: z.ZipFile,
				path:    fpath,
				file:    f,
				fs:      z.fs,
			}
			return zm, nil

		case f.Name[:len(f.Name)-1] == path && f.Name[len(f.Name)-1] == '/':
			zd := &ZipDir{
				ZipFile: z.ZipFile,
				path:    f.Name,
				fs:      z.fs,
			}
			return zd, nil
		}
	}

	log.Error("Failed to lookup", "path", path, "ZipDir", z)
	return nil, fuse.ENOENT
}

type ZipMember struct {
	*ZipFile
	path string
	fs   *FS
	file *zip.File
	fp   io.ReadCloser
}

func (z *ZipMember) String() string {
	return fmt.Sprintf("ZipMember<%s>", z.path)
}

func (z *ZipMember) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Valid = 1 * time.Hour
	attr.Inode = z.fs.GetInode(z.path)
	attr.Size = z.file.UncompressedSize64
	attr.Mode = z.file.Mode()
	attr.Mtime = z.file.ModTime()
	return nil
}

func (z *ZipMember) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	fp, err := z.file.Open()
	if err != nil {
		log.Error("Failed to open zipmember", "ZipMember", z, "error", err)
		return nil, err
	}

	z.fp = fp
	return z, nil
}

func (z *ZipMember) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := z.fp.Close(); err != nil {
		log.Error("Failed to close fp", "ZipMember", z, "error", err)
		return err
	}

	return nil
}

func (z *ZipMember) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Debug("ZipMember.Read", "ZipMember", z, "offset", req.Offset, "size", req.Size)
	buf := make([]byte, req.Size)
	// TODO if seeking isn't allowed, then this part is unnecessary
	if req.Offset != 0 {
		throwaway := make([]byte, req.Offset)
		z.fp.Read(throwaway)
	}

	n, err := z.fp.Read(buf)
	resp.Data = buf[:n]
	return err
}

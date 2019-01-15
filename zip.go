package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
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
	ig   InodeGenerator
	icd  ImageConverterDetector
}

func (z *ZipDir) String() string {
	return fmt.Sprintf("ZipDir<%s/%s>", z.ZipFile.path, z.path)
}

func (z *ZipDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Valid = 1 * time.Hour
	attr.Inode = z.ig.GenerateInode(z.ZipFile.path + z.path)
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

func (z *ZipDir) Clone() *ZipDir {
	zd := &ZipDir{
		ZipFile: z.ZipFile,
		path:    z.path,
		ig:      z.ig,
		icd:     z.icd,
	}
	return zd
}

func (z *ZipDir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	if err := z.EnsureArchiveOpen(); err != nil {
		return nil, err
	}

	name := req.Name
	var imageConverter ImageConverter

	// Lookups might be of the form *.webp.png where only *.webp exists and needs
	// to be converted to png. The following code cleans up the request name
	if newName, imgConv := z.icd.Detect(name); imgConv != nil {
		log.Debug("imgconv detected", "name", name, "conv", imgConv)
		name = newName
		imageConverter = imgConv
	}

	path := filepath.Join(z.path, name)
	for _, f := range z.archive.File {
		switch {
		case f.Name == path:
			fpath := filepath.Join(z.path, f.Name)
			zm := &ZipMember{
				ZipFile: z.ZipFile,
				path:    fpath,
				file:    f,
				ig:      z.ig,
				ic:      imageConverter,
			}
			return zm, nil

		case f.Name[:len(f.Name)-1] == path && f.Name[len(f.Name)-1] == '/':
			zd := z.Clone()
			z.path = f.Name
			return zd, nil
		}
	}

	log.Error("Failed to lookup", "path", path, "ZipDir", z)
	return nil, fuse.ENOENT
}

type ZipMember struct {
	*ZipFile
	path string
	ig   InodeGenerator
	ic   ImageConverter
	file *zip.File
	fp   io.ReadCloser
	data []byte
}

func (z *ZipMember) String() string {
	return fmt.Sprintf("ZipMember<%s/%s>", z.ZipFile.path, z.path)
}

func (z *ZipMember) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Valid = 2 * time.Minute
	attr.Inode = z.ig.GenerateInode(z.ZipFile.path + z.path)
	attr.Size = z.file.UncompressedSize64
	attr.Mode = z.file.Mode()
	attr.Mtime = z.file.ModTime()

	fp, err := z.file.Open()
	if err != nil {
		log.Error("Failed to open zipmember", "ZipMember", z, "error", err)
		return err
	}

	data, err := ioutil.ReadAll(fp)
	if err != nil {
		return err
	}

	z.data = data
	if z.ic != nil {
		if converted, err := z.ic.Convert(z.data); err != nil {
			log.Error("Failed to convert image", "ic", z.ic, "error", err)
		} else {
			z.data = converted
		}
	}

	attr.Size = uint64(len(z.data))

	if err := fp.Close(); err != nil {
		log.Error("Failed to close fp", "ZipMember", z, "error", err)
		return err
	}

	return nil
}

func (z *ZipMember) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Debug("ZipMember.Read", "ZipMember", z, "offset", req.Offset, "size", req.Size)
	end := int(req.Offset) + req.Size
	if len(z.data) < end {
		end = len(z.data)
	}
	resp.Data = z.data[req.Offset:end]
	return nil
}

func ZipHandlerCreator(path string, ig InodeGenerator) (ComicHandler, error) {
	zf := &ZipFile{path: path}
	if err := zf.Init(); err != nil {
		log.Error("Failed to initialize zipfile", "path", path, "error", err)
		return nil, err
	}

	icd := &SimpleImageConverter{}
	icd.Init()

	return &ZipDir{ZipFile: zf, path: "", ig: ig, icd: icd}, nil
}

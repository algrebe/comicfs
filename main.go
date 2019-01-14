package main

import (
	"flag"
	"fmt"
	log "github.com/inconshreveable/log15"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	//_ "bazil.org/fuse/fs/fstestutil"
)

var progName = filepath.Base(os.Args[0])

func mount(comicDir, mountpoint string) error {
	if _, err := os.Stat(comicDir); os.IsNotExist(err) {
		log.Error("comicdir does not exist", "path", comicDir)
		return err
	}

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("comicfs"),
		fuse.ReadOnly(),
		fuse.Subtype("comicfs"),
	)

	if err != nil {
		return err
	}
	defer c.Close()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		for _ = range sigChan {
			log.Info("Got interrupt signal, trying to unmount")
			if err := fuse.Unmount(mountpoint); err != nil {
				log.Error("Failed to unmount", "error", err)
			} else {
				log.Info("Unmounted successfully")
				time.Sleep(1 * time.Second)
				os.Exit(1)
			}
		}
	}()

	ct := make(map[string]struct{})
	ct[".cbz"] = struct{}{}
	ct[".zip"] = struct{}{}

	filesys := &FS{
		ComicDir:   comicDir,
		ComicTypes: ct,
	}

	if err := filesys.Init(); err != nil {
		return err
	}

	if err := fs.Serve(c, filesys); err != nil {
		return err
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}

	return nil
}

func setLogLvl(s string) error {
	lvl, err := log.LvlFromString(s)
	if err != nil {
		return err
	}

	log.Root().SetHandler(log.LvlFilterHandler(lvl, log.StdoutHandler))
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s [options] comic_dir mountpoint\n", progName)
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	logLevel := flag.String("log-level", "info", "displays all logs at or above this level")
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}

	if err := setLogLvl(*logLevel); err != nil {
		panic(err)
	}

	comicDir := flag.Arg(0)
	mountpoint := flag.Arg(1)

	if err := mount(comicDir, mountpoint); err != nil {
		log.Error("Failed to mount", "error", err)
		os.Exit(2)
	}
}

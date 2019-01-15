package main

import (
	"flag"
	"fmt"
	log "github.com/inconshreveable/log15"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

func mount(comicDir, mountpoint string, filesystem fs.FS) error {
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

	if err := fs.Serve(c, filesystem); err != nil {
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
	progName := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s [options] comic_dir mountpoint\n", progName)
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	logLevel := flag.String("log-level", "info", "displays all logs at or above this level")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "write memory profile to `file`")
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
		os.Exit(1)
	}

	if err := setLogLvl(*logLevel); err != nil {
		panic(err)
	}

	comicDir := flag.Arg(0)
	mountpoint := flag.Arg(1)

	if _, err := os.Stat(comicDir); os.IsNotExist(err) {
		log.Error("comicdir does not exist", "path", comicDir)
		os.Exit(1)
	}

	filesys := &FS{
		ComicDir: comicDir,
	}

	filesys.Init()

	filesys.RegisterComicType(".zip", ZipHandlerCreator)
	filesys.RegisterComicType(".cbz", ZipHandlerCreator)

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Error("failed to create CPU profile", "error", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Error("Failed to start CPU profile", "error", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	if err := mount(comicDir, mountpoint, filesys); err != nil {
		log.Error("Failed to mount", "error", err)
		os.Exit(1)
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Error("Failed to create memory profile", "error", err)
			os.Exit(1)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Error("Failed to write memory profile", "error", err)
			os.Exit(1)
		}
		f.Close()
	}
}

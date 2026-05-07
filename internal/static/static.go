package static

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
)

//go:embed dist/*
var embeddedWebFS embed.FS

func GetEmbeddedWebFS(modTime *time.Time) (fs.FS, error) {

	if os.Getenv("DEBUG") == "1" {
		// List all embedded static files at startup for debugging purposes
		fmt.Println("Embedded static files:")
		if err := fs.WalkDir(embeddedWebFS, "dist", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("  [error] %s: %v\n", path, err)
				return nil
			}

			if d.IsDir() {
				fmt.Printf("  %s/\n", path)
			} else {
				fmt.Printf("  %s\n", path)
			}

			return nil
		}); err != nil {
			fmt.Printf("failed to walk embedded fs: %v\n", err)
		}
	}

	dist, err := fs.Sub(embeddedWebFS, "dist")
	if err != nil {
		return nil, err
	}
	t := modTime
	if t == nil {
		t = models.Now()
	}
	return &StaticFSWrapper{
		FS:           dist,
		FixedModTime: *t,
	}, nil
}

// Wrapper around embed.FS to return a fixed mod time for all files, so that caching works properly in browsers.
// Without this, the mod time is 0 (unix epoch) which causes browsers to treat the files as always stale and not cache them.
type StaticFSWrapper struct {
	fs.FS
	FixedModTime time.Time
}

func (f *StaticFSWrapper) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)

	return &StaticFileWrapper{File: file, fixedModTime: f.FixedModTime}, err
}

type StaticFileWrapper struct {
	fs.File
	fixedModTime time.Time
}

func (f *StaticFileWrapper) Stat() (os.FileInfo, error) {
	fileInfo, err := f.File.Stat()
	return &StaticFileInfoWrapper{FileInfo: fileInfo, fixedModTime: f.fixedModTime}, err
}

type StaticFileInfoWrapper struct {
	os.FileInfo
	fixedModTime time.Time
}

func (f *StaticFileInfoWrapper) ModTime() time.Time {
	return f.fixedModTime
}

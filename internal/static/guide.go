package static

import (
	"embed"
	"io/fs"
	"time"
)

// The parent directory contains a tracked placeholder so Go can compile before
// generation. make run/build/test generate dist before starting the app.
//
//go:embed all:guide
var embeddedGuideFS embed.FS

func GetEmbeddedGuideFS(modTime *time.Time) (fs.FS, error) {
	dist, err := fs.Sub(embeddedGuideFS, "guide/dist")
	if err != nil {
		return nil, err
	}
	buildTime := time.Now()
	if modTime != nil {
		buildTime = *modTime
	}
	return &StaticFSWrapper{FS: dist, FixedModTime: buildTime}, nil
}

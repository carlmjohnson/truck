package main // import "github.com/carlmjohnson/truck"

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karrick/godirwalk"
	"github.com/rwcarlsen/goexif/exif"
)

func main() {
	if err := exec(); err != nil {
		log.Fatal(err)
	}
}

func exec() error {
	flag.Parse()
	root := flag.Arg(0)

	matchDir := func(path string) bool {
		return !strings.HasPrefix(path, ".")
	}
	matchFile := func(path string) bool {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".jpeg" || ext == ".jpg"
	}
	paths, err := listFiles(root, matchDir, matchFile)
	if err != nil {
		return err
	}

	for _, path := range paths {
		date, err := getEXIFDate(path)
		if err != nil {
			fmt.Fprint(os.Stderr, "err at %q: %v", path, err)
			continue
		}
		dir := filepath.Dir(path)
		newpath := filepath.Join(dir, fmt.Sprintf("%d.jpg", date.Unix()))
		if newpath == path {
			continue
		}
		if _, err := os.Stat(newpath); !os.IsNotExist(err) {
			fmt.Fprint(os.Stderr, "cannot rename %q to %q", path, newpath)
			continue
		}
		if err = os.Rename(path, newpath); err != nil {
			fmt.Fprint(os.Stderr, "err at %q: %v", path, err)
		}
	}
	return nil
}

func listFiles(root string, matchDir, matchFile func(string) bool) (paths []string, err error) {
	err = godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(path string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				if !matchDir(path) {
					return filepath.SkipDir
				}
			} else {
				if matchFile(path) {
					paths = append(paths, path)
				}
			}
			return nil
		},
	})
	return
}

func getEXIFDate(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, err
	}
	return x.DateTime()
}
